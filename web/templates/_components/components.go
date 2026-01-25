package components

import "html/template"

type CardData struct {
	ID           string
	Header       string
	HeaderAction template.HTML
	Content      template.HTML
	Footer       template.HTML
	Clickable    bool
	Href         string
}

type TableData struct {
	Headers []string
	Rows    [][]string
	Search  bool
}

type SidebarItem struct {
	Label  string
	Href   string
	Active bool
}

type SidebarData struct {
	Title    string
	Position string // left | right
	Mode     string // fixed | overlay | dropdown
	Items    []SidebarItem
}

type NavItem struct {
	Label    string
	Href     string
	Active   bool
	Children []NavItem
}

type NavbarData struct {
	Brand           string
	Items           []NavItem
	User            *User
	ShowThemeToggle bool
}
