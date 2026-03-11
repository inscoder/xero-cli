package version

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
	Date    string `json:"date,omitempty"`
}

func Current() Info {
	commit := Commit
	if commit == "none" {
		commit = ""
	}

	date := Date
	if date == "unknown" {
		date = ""
	}

	return Info{
		Version: Version,
		Commit:  commit,
		Date:    date,
	}
}
