//! HTTP handlers — the only file in the TCB that touches axum.
//!
//! Every handler:
//!   1. Decodes the request via `axum::Json` / `axum::extract::Query`.
//!   2. Calls one verified `service::Service` method.
//!   3. Encodes the response with `axum::Json` and an explicit status code.
//!
//! The error-code mapping mirrors the Go impl byte-for-byte so the
//! conformance suite passes against either implementation.

use std::sync::Arc;

use axum::{
    body::Bytes,
    extract::{RawQuery, State},
    http::StatusCode,
    middleware,
    response::IntoResponse,
    routing::{get, post},
    Json, Router,
};
use serde::{Deserialize, Serialize};

use service::{Service, ServiceError};

use crate::metrics;

/// Construct the axum router around `svc`.
pub fn router(svc: Arc<Service>) -> Router {
    let api = Router::new()
        .route("/users", post(create_user).fallback(not_found))
        .route("/follow", post(follow).delete(unfollow).fallback(not_found))
        .route("/tweets", post(post_tweet).fallback(not_found))
        .route("/timeline", get(timeline).fallback(not_found))
        .route("/tick", post(tick).fallback(not_found))
        .route("/healthz", get(healthz))
        .route("/version", get(version))
        .route("/metrics", get(metrics::render));
    // The demo UI owns "/" and is NOT part of the observable contract, so it
    // is opt-in. With it mounted, an unrouted path reaches the UI instead of
    // the JSON not_found that totality requires, and R0 fails. Default is
    // API-only, matching the Go corner.
    let with_ui = if std::env::var("UI").as_deref() == Ok("true") {
        crate::ui::mount(api)
    } else {
        api
    };
    crate::admin::mount(with_ui)
        .fallback(not_found)
        .layer(middleware::from_fn(metrics::track))
        .with_state(svc)
}

// -----------------------------------------------------------------------------
// GET /healthz — liveness/readiness probe used by Fly's load balancer
// (Tier-4 phase 1)
// -----------------------------------------------------------------------------

async fn healthz() -> impl IntoResponse {
    (StatusCode::OK, "ok\n")
}

// -----------------------------------------------------------------------------
// GET /version — image provenance (Tier-4 phase 1; baked into image at build time)
//
// Reads `/etc/version.json` written by the build-image-main job in
// verify.yml. On rollback the previous image is re-pulled, so /version
// automatically reports the rolled-back release's digest — no env-var
// sync required (K21 fix from the converged Tier-4 plan).
// -----------------------------------------------------------------------------

#[derive(Serialize)]
struct VersionResp {
    git_sha: String,
    image_digest: String,
    process_uptime_seconds: u64,
    snapshot_version: u32,
}

async fn version() -> impl IntoResponse {
    static START: std::sync::OnceLock<std::time::Instant> = std::sync::OnceLock::new();
    let start = START.get_or_init(std::time::Instant::now);
    let uptime = start.elapsed().as_secs();

    // Defaults: dev/local runs without a baked image.
    let mut git_sha = String::from("dev");
    let mut image_digest = String::from("sha256:dev");

    if let Ok(s) = std::fs::read_to_string("/etc/version.json") {
        if let Ok(v) = serde_json::from_str::<serde_json::Value>(&s) {
            if let Some(g) = v.get("git_sha").and_then(|x| x.as_str()) {
                git_sha = g.to_string();
            }
            if let Some(d) = v.get("image_digest").and_then(|x| x.as_str()) {
                image_digest = d.to_string();
            }
        }
    }

    Json(VersionResp {
        git_sha,
        image_digest,
        process_uptime_seconds: uptime,
        snapshot_version: 1,
    })
}

#[derive(Serialize)]
struct ErrBody {
    error: &'static str,
}

fn err(status: StatusCode, code: &'static str) -> (StatusCode, Json<ErrBody>) {
    (status, Json(ErrBody { error: code }))
}

fn map_err(e: ServiceError) -> (StatusCode, &'static str) {
    match e {
        ServiceError::InvalidHandle => (StatusCode::BAD_REQUEST, "invalid_handle"),
        ServiceError::InvalidText => (StatusCode::BAD_REQUEST, "invalid_text"),
        ServiceError::SelfFollow => (StatusCode::BAD_REQUEST, "self_follow_forbidden"),
        ServiceError::UnknownUser => (StatusCode::BAD_REQUEST, "unknown_user"),
        ServiceError::HandleTaken => (StatusCode::CONFLICT, "handle_taken"),
        ServiceError::NonMonotonic => (StatusCode::INTERNAL_SERVER_ERROR, "internal_error"),
    }
}

/// Parses exactly one JSON object, rejecting unknown fields and trailing
/// content (S_obs decision D7).
///
/// Written by hand rather than using axum's `Json` extractor because that
/// extractor owns its own rejection response, and the observable contract
/// fixes the body to `{"error":"malformed_request"}`. Lenient parsing is a
/// classic source of cross-language divergence -- ten of the 54 R0 baseline
/// steps failed here on both implementations.
fn parse_strict<T: for<'de> Deserialize<'de>>(body: &Bytes) -> Option<T> {
    serde_json::from_slice::<T>(body).ok()
}

fn malformed() -> axum::response::Response {
    err(StatusCode::BAD_REQUEST, "malformed_request").into_response()
}

/// Total-by-construction default route (D7). Without it axum answers an
/// unrouted path or an unmatched method with an empty body, which is not the
/// observable contract.
async fn not_found() -> impl IntoResponse {
    err(StatusCode::NOT_FOUND, "not_found")
}

/// Rejects a query string on a route that does not take one.
///
/// axum's Router matches on the path and ignores the query entirely, so
/// POST /users?x=1 and POST /tweets?limit=1 reached their handlers. S_obs
/// routes on (method, path) with NO query for every endpoint except
/// /timeline, so those are not routes at all.
///
/// Found by widening the trace generator's alphabet, not by volume: 100,000
/// generated requests never emitted a query on a POST path. See finding F008.
fn reject_query(raw: &Option<String>) -> Option<axum::response::Response> {
    match raw {
        Some(q) if !q.is_empty() => Some(err(StatusCode::NOT_FOUND, "not_found").into_response()),
        _ => None,
    }
}

// -----------------------------------------------------------------------------
// POST /users
// -----------------------------------------------------------------------------

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct CreateUserReq {
    handle: String,
}

#[derive(Serialize)]
struct UserResp {
    handle: String,
    id: i64,
}

async fn create_user(
    State(svc): State<Arc<Service>>,
    RawQuery(rq): RawQuery,
    body: Bytes,
) -> impl IntoResponse {
    if let Some(r) = reject_query(&rq) {
        return r;
    }
    let Some(req) = parse_strict::<CreateUserReq>(&body) else {
        return malformed();
    };
    match svc.create_user(&req.handle) {
        Ok(u) => (StatusCode::CREATED, Json(UserResp { handle: u.handle, id: u.id }))
            .into_response(),
        Err(e) => {
            let (status, code) = map_err(e);
            metrics::note_violation(code, "/users");
            err(status, code).into_response()
        }
    }
}

// -----------------------------------------------------------------------------
// POST /follow, DELETE /follow
// -----------------------------------------------------------------------------

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct FollowReq {
    from: String,
    to: String,
}

async fn follow(
    State(svc): State<Arc<Service>>,
    RawQuery(rq): RawQuery,
    body: Bytes,
) -> impl IntoResponse {
    if let Some(r) = reject_query(&rq) {
        return r;
    }
    let Some(req) = parse_strict::<FollowReq>(&body) else {
        return malformed();
    };
    match svc.follow(&req.from, &req.to) {
        Ok(()) => StatusCode::NO_CONTENT.into_response(),
        Err(e) => {
            let (status, code) = map_err(e);
            metrics::note_violation(code, "/follow");
            err(status, code).into_response()
        }
    }
}

async fn unfollow(
    State(svc): State<Arc<Service>>,
    RawQuery(rq): RawQuery,
    body: Bytes,
) -> impl IntoResponse {
    if let Some(r) = reject_query(&rq) {
        return r;
    }
    let Some(req) = parse_strict::<FollowReq>(&body) else {
        return malformed();
    };
    match svc.unfollow(&req.from, &req.to) {
        Ok(()) => StatusCode::NO_CONTENT.into_response(),
        Err(e) => {
            let (status, code) = map_err(e);
            metrics::note_violation(code, "/follow");
            err(status, code).into_response()
        }
    }
}

// -----------------------------------------------------------------------------
// POST /tweets
// -----------------------------------------------------------------------------

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct TweetReq {
    author: String,
    text: String,
}

#[derive(Serialize)]
struct TweetResp {
    id: i64,
    author: String,
    text: String,
    created_at: i64,
}

async fn post_tweet(
    State(svc): State<Arc<Service>>,
    RawQuery(rq): RawQuery,
    body: Bytes,
) -> impl IntoResponse {
    if let Some(r) = reject_query(&rq) {
        return r;
    }
    let Some(req) = parse_strict::<TweetReq>(&body) else {
        return malformed();
    };
    match svc.post_tweet(&req.author, &req.text) {
        Ok(t) => (
            StatusCode::CREATED,
            Json(TweetResp { id: t.id, author: t.author, text: t.text, created_at: t.created_at }),
        )
            .into_response(),
        Err(e) => {
            let (status, code) = map_err(e);
            metrics::note_violation(code, "/tweets");
            err(status, code).into_response()
        }
    }
}

// -----------------------------------------------------------------------------
// POST /tick
// -----------------------------------------------------------------------------

#[derive(Serialize)]
struct ClockResp {
    clock: i64,
}

/// Advances the logical clock (S_obs decision D3).
///
/// The clock previously had no route. That is why the shared corpus asserted a
/// created_at no sequence of its own requests could reach, and why both
/// conformance harnesses resolved it by writing to the clock directly -- see
/// evidence/findings/F001. One request now maps 1:1 onto one TLA+ Tick step.
async fn tick(
    State(svc): State<Arc<Service>>,
    RawQuery(rq): RawQuery,
    body: Bytes,
) -> impl IntoResponse {
    if let Some(r) = reject_query(&rq) {
        return r;
    }
    // No trim: S_obs accepts exactly "" or "{}" (D3), so " {} " is malformed.
    let s = String::from_utf8_lossy(&body);
    if !s.is_empty() && s != "{}" {
        return malformed();
    }
    svc.tick();
    (StatusCode::OK, Json(ClockResp { clock: svc.now() })).into_response()
}

// -----------------------------------------------------------------------------
// GET /timeline
// -----------------------------------------------------------------------------

#[derive(Serialize)]
struct TimelineResp {
    tweets: Vec<TweetResp>,
    next_cursor: Option<i64>,
}

/// Parsed by hand from the raw query rather than via `Query<T>`, so that
/// unknown and repeated parameters are rejected explicitly (D7) instead of
/// being silently taken-first or rejected with the extractor's own message.
/// Those two behaviours are exactly where the Go and Rust implementations
/// disagreed with each other before retargeting -- see finding F006.
async fn timeline(
    State(svc): State<Arc<Service>>,
    RawQuery(raw): RawQuery,
) -> impl IntoResponse {
    let Some(raw) = raw.filter(|r| !r.is_empty()) else {
        return malformed();
    };
    // A malformed percent-escape makes the whole query malformed. The first
    // version of parse_query decoded "%zz" to the literal text and carried on,
    // so ?limit=%zz surfaced as invalid_limit instead of malformed_request --
    // a wrong answer that still looked like a rejection. Go's url.ParseQuery
    // returns an error here and S_obs propagates it. (F008)
    let Some(pairs) = parse_query(&raw) else {
        return malformed();
    };
    let mut user: Option<String> = None;
    let mut limit_raw: Option<String> = None;
    let mut cursor_raw: Option<String> = None;
    for (k, v) in pairs {
        let slot = match k.as_str() {
            "user" => &mut user,
            "limit" => &mut limit_raw,
            "cursor" => &mut cursor_raw,
            _ => return malformed(),
        };
        if slot.is_some() {
            return malformed(); // repeated parameter
        }
        *slot = Some(v);
    }
    let Some(user) = user else { return malformed() };
    if !domain::valid_handle(&user) {
        return err(StatusCode::BAD_REQUEST, "invalid_handle").into_response();
    }
    if !svc.has_user(&user) {
        metrics::note_violation("unknown_user", "/timeline");
        return err(StatusCode::BAD_REQUEST, "unknown_user").into_response();
    }
    let limit = match limit_raw {
        None => 50usize,
        Some(s) => match s.parse::<i64>() {
            Ok(v) if (1..=100).contains(&v) => v as usize,
            _ => return err(StatusCode::BAD_REQUEST, "invalid_limit").into_response(),
        },
    };
    let cursor = match cursor_raw {
        None => 0i64,
        Some(s) => match s.parse::<i64>() {
            Ok(v) if v >= 1 => v,
            _ => return err(StatusCode::BAD_REQUEST, "invalid_cursor").into_response(),
        },
    };

    let (tw, more) = svc.home_timeline(&user, limit, cursor);
    let next_cursor = if more { tw.last().map(|t| t.id) } else { None };
    let resp = TimelineResp {
        tweets: tw
            .into_iter()
            .map(|t| TweetResp { id: t.id, author: t.author, text: t.text, created_at: t.created_at })
            .collect(),
        next_cursor,
    };
    (StatusCode::OK, Json(resp)).into_response()
}

/// Minimal `application/x-www-form-urlencoded` parser.
///
/// Hand-written rather than pulling in a crate: the grammar accepted here is
/// three known keys with scalar values, and the observable contract rejects
/// anything else outright, so the parser's job is to split and percent-decode.
/// Preserving repeated keys (rather than collapsing them) is required -- D7
/// rejects them, and silently taking the first value is precisely where the Go
/// and Rust implementations disagreed before retargeting (finding F006).
fn parse_query(raw: &str) -> Option<Vec<(String, String)>> {
    raw.split('&')
        .filter(|p| !p.is_empty())
        .map(|pair| {
            let (k, v) = pair.split_once('=').unwrap_or((pair, ""));
            Some((percent_decode(k)?, percent_decode(v)?))
        })
        .collect()
}

/// Percent-decodes, returning None on a malformed escape.
///
/// Matches Go's url.ParseQuery: a '%' not followed by two hex digits, or a
/// trailing '%', makes the query invalid rather than decoding to itself.
/// Note the bound is `i + 2 < b.len()` in the original, which also silently
/// mishandled an escape at the very end of the string.
fn percent_decode(s: &str) -> Option<String> {
    let b = s.as_bytes();
    let mut out = Vec::with_capacity(b.len());
    let mut i = 0;
    while i < b.len() {
        match b[i] {
            b'+' => {
                out.push(b' ');
                i += 1;
            }
            b'%' => {
                if i + 2 >= b.len() {
                    return None;
                }
                let v = u8::from_str_radix(&s[i + 1..i + 3], 16).ok()?;
                out.push(v);
                i += 3;
            }
            c => {
                out.push(c);
                i += 1;
            }
        }
    }
    Some(String::from_utf8_lossy(&out).into_owned())
}
