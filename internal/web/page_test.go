package web

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJobPageShowsGotAsSVG(t *testing.T) {
	var buf bytes.Buffer
	err := JobPage(JobView{ID: "job1", Status: "running"}).Render(t.Context(), &buf)
	require.NoError(t, err)
	body := buf.String()
	require.Contains(t, body, `/jobs/job1/files/last.svg`)
	require.Contains(t, body, `["got", "last.svg"]`)
	require.NotContains(t, body, "last.png")
	require.Contains(t, body, `row.slot`)
	require.Contains(t, body, `badge.textContent = "slot"`)
}
