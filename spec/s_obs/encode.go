package sobs

import (
	"sort"
	"strconv"
	"strings"
)

// Canonical JSON encoding.
//
// Byte-identical replay across four languages requires the response encoding
// to be pinned, not left to each language's default JSON writer. The rules:
//
//	R1  Object keys appear in the fixed order declared by each writer below,
//	    NOT alphabetically. The order is part of the contract.
//	R2  No insignificant whitespace.
//	R3  Integers only. No floating point anywhere in the response surface.
//	R4  Strings escape only what RFC 8259 requires, plus \b \f \n \r \t as
//	    short forms. No \u escaping of non-ASCII, no HTML escaping of < > &.
//	R5  null is written literally as null, never omitted.
//
// Implementations that emit a different byte sequence for the same logical
// response FAIL R0, by design.

func encodeString(sb *strings.Builder, s string) {
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\b':
			sb.WriteString(`\b`)
		case '\f':
			sb.WriteString(`\f`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		default:
			if r < 0x20 {
				sb.WriteString(`\u`)
				const hex = "0123456789abcdef"
				sb.WriteByte('0')
				sb.WriteByte('0')
				sb.WriteByte(hex[(r>>4)&0xF])
				sb.WriteByte(hex[r&0xF])
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
}

func encodeInt(sb *strings.Builder, n int64) {
	sb.WriteString(strconv.FormatInt(n, 10))
}

// errorBody renders {"error":"<code>"}.
func errorBody(code string) string {
	var sb strings.Builder
	sb.WriteString(`{"error":`)
	encodeString(&sb, code)
	sb.WriteByte('}')
	return sb.String()
}

// userBody renders {"handle":"<h>","id":<n>}.
func userBody(u User) string {
	var sb strings.Builder
	sb.WriteString(`{"handle":`)
	encodeString(&sb, u.Handle)
	sb.WriteString(`,"id":`)
	encodeInt(&sb, u.ID)
	sb.WriteByte('}')
	return sb.String()
}

// tweetObject renders {"id":<n>,"author":"<h>","text":"<t>","created_at":<n>}.
func tweetObject(sb *strings.Builder, t Tweet) {
	sb.WriteString(`{"id":`)
	encodeInt(sb, t.ID)
	sb.WriteString(`,"author":`)
	encodeString(sb, t.Author)
	sb.WriteString(`,"text":`)
	encodeString(sb, t.Text)
	sb.WriteString(`,"created_at":`)
	encodeInt(sb, t.CreatedAt)
	sb.WriteByte('}')
}

func tweetBody(t Tweet) string {
	var sb strings.Builder
	tweetObject(&sb, t)
	return sb.String()
}

// timelineBody renders {"tweets":[...],"next_cursor":<n>|null}.
func timelineBody(page []Tweet, nextCursor *int64) string {
	var sb strings.Builder
	sb.WriteString(`{"tweets":[`)
	for i, t := range page {
		if i > 0 {
			sb.WriteByte(',')
		}
		tweetObject(&sb, t)
	}
	sb.WriteString(`],"next_cursor":`)
	if nextCursor == nil {
		sb.WriteString("null")
	} else {
		encodeInt(&sb, *nextCursor)
	}
	sb.WriteByte('}')
	return sb.String()
}

// clockBody renders {"clock":<n>}.
func clockBody(n int64) string {
	var sb strings.Builder
	sb.WriteString(`{"clock":`)
	encodeInt(&sb, n)
	sb.WriteByte('}')
	return sb.String()
}

func sortEdges(e []Edge) {
	sort.Slice(e, func(i, j int) bool {
		if e[i].From != e[j].From {
			return e[i].From < e[j].From
		}
		return e[i].To < e[j].To
	})
}
