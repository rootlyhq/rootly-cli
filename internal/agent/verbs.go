package agent

var statusVerbs = []struct {
	Active string
	Done   string
}{
	{Active: "Triaging", Done: "Triaged"},
	{Active: "Investigating", Done: "Investigated"},
	{Active: "Correlating", Done: "Correlated"},
	{Active: "Untangling", Done: "Untangled"},
	{Active: "Spelunking", Done: "Spelunked"},
	{Active: "Sleuthing", Done: "Sleuthed"},
	{Active: "Wrangling", Done: "Wrangled"},
	{Active: "Deciphering", Done: "Deciphered"},
	{Active: "Escalating thoughts", Done: "Escalated thoughts"},
	{Active: "Paging the hamsters", Done: "Paged the hamsters"},
	{Active: "Rummaging through logs", Done: "Rummaged through logs"},
	{Active: "Reticulating runbooks", Done: "Reticulated runbooks"},
}
