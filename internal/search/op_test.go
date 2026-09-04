package search

import (
	"encoding/json"
	"testing"
)

func TestOpJSONIsName(t *testing.T) {
	raw, err := json.Marshal(Rated{Op: OpTriangle, Ok: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"name":"triangle","score":null,"ok":true}` {
		t.Fatalf("json=%s", raw)
	}
	var got Rated
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Op != OpTriangle || !got.Ok {
		t.Fatalf("got=%+v", got)
	}
}

func TestParseOp(t *testing.T) {
	op, err := ParseOp("rectangle")
	if err != nil || op != OpRectangle {
		t.Fatalf("op=%v err=%v", op, err)
	}
	if _, err := ParseOp("nope"); err == nil {
		t.Fatal("expected error")
	}
}
