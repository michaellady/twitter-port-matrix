package main

import "encoding/json"

func jsonInto(body string, v any) error {
	return json.Unmarshal([]byte(body), v)
}
