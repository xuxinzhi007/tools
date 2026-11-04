package devops

import "strings"
import "testing"

func TestLinesFromContentAndObjects(t *testing.T) {
	payload := []byte(`{"data":{"logs":[
		{"content":"{\"level\":\"INFO\",\"msg\":\"[ListTags] req={}\"}"},
		{"level":"ERROR","msg":"SQL Error","sql":"SELECT 1"}
	]}}`)
	lines, err := Lines(payload)
	if err != nil || len(lines) != 2 || !strings.Contains(lines[0], "ListTags") || !strings.Contains(lines[1], "SQL Error") {
		t.Fatalf("lines: %v %v", lines, err)
	}
}
