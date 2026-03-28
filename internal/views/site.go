package views

// Site configuration
const (
	SiteName    = "stashfor.me"
	SiteTagline = "Save and share links via SMS"
)

// PageTitle returns a formatted page title
func PageTitle(page string) string {
	if page == "" {
		return SiteName
	}
	return page + " - " + SiteName
}
