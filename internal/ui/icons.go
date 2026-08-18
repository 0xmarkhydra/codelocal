package ui

func icon(path string) string {
	return `<svg viewBox="0 0 24 24" aria-hidden="true">` + path + `</svg>`
}

var navIcons = map[string]string{
	"overview":    icon(`<rect x="3.5" y="3.5" width="7" height="7" rx="2"/><rect x="13.5" y="3.5" width="7" height="7" rx="2"/><rect x="3.5" y="13.5" width="7" height="7" rx="2"/><rect x="13.5" y="13.5" width="7" height="7" rx="2"/>`),
	"workspaces":  icon(`<path d="M3.5 7.5a2 2 0 0 1 2-2h4l2 2h7a2 2 0 0 1 2 2v8.5a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z"/><path d="M3.5 10h17"/>`),
	"knowledge":   icon(`<circle cx="5" cy="12" r="2.2"/><circle cx="12" cy="5" r="2.2"/><circle cx="19" cy="12" r="2.2"/><circle cx="12" cy="19" r="2.2"/><path d="m6.6 10.4 3.8-3.8m3.2 0 3.8 3.8m0 3.2-3.8 3.8m-3.2 0-3.8-3.8"/>`),
	"codegraph":   icon(`<circle cx="12" cy="12" r="2.4"/><circle cx="5" cy="6" r="1.8"/><circle cx="19" cy="6" r="1.8"/><circle cx="5" cy="18" r="1.8"/><circle cx="19" cy="18" r="1.8"/><path d="m6.5 7.3 3.6 3.1m3.8 0 3.6-3.1m-11 9.4 3.6-3.1m3.8 0 3.6 3.1"/>`),
	"devices":     icon(`<rect x="4" y="4" width="16" height="11" rx="2.5"/><path d="M2.5 19h19M9 15v4M15 15v4"/>`),
	"usage":       icon(`<path d="M4 18V10M9.5 18V6M15 18v-5M20 18V4"/>`),
	"connect":     icon(`<path d="m9.5 14.5-1 1a4 4 0 0 1-5.7-5.7l3-3a4 4 0 0 1 5.7 0"/><path d="m14.5 9.5 1-1a4 4 0 0 1 5.7 5.7l-3 3a4 4 0 0 1-5.7 0"/><path d="m8.5 15.5 7-7"/>`),
	"invite":      icon(`<circle cx="9" cy="8" r="3"/><path d="M3.5 20c.6-3.8 2.6-5.7 5.5-5.7 1.6 0 2.9.5 3.8 1.4"/><path d="M17.5 11.5v7M14 15h7"/>`),
	"leaderboard": icon(`<path d="M8 4h8v3.5a4 4 0 0 1-8 0z"/><path d="M8 6H5v1.5a4 4 0 0 0 4 4M16 6h3v1.5a4 4 0 0 1-4 4M12 11.5V16M8.5 20h7M9 16h6"/>`),
	"security":    icon(`<path d="M12 3.2 19 6v5c0 4.5-2.7 8-7 9.8C7.7 19 5 15.5 5 11V6z"/><path d="m9 12 2 2 4-4"/>`),
	"admin":       icon(`<path d="M12 3.2 19 6v5c0 4.5-2.7 8-7 9.8C7.7 19 5 15.5 5 11V6z"/><circle cx="12" cy="9" r="2"/><path d="M8.8 15c.7-1.7 1.8-2.5 3.2-2.5s2.5.8 3.2 2.5"/>`),
}

var UIIcons = map[string]string{
	"overview":     navIcons["overview"],
	"workspaces":   navIcons["workspaces"],
	"knowledge":    navIcons["knowledge"],
	"codegraph":    navIcons["codegraph"],
	"devices":      navIcons["devices"],
	"usage":        navIcons["usage"],
	"connect":      navIcons["connect"],
	"invite":       navIcons["invite"],
	"leaderboard":  navIcons["leaderboard"],
	"security":     navIcons["security"],
	"account":      navIcons["security"],
	"admin":        navIcons["admin"],
	"folder":       icon(`<path d="M3.5 7.5a2 2 0 0 1 2-2h4l2 2h7a2 2 0 0 1 2 2v8.5a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z"/><path d="M3.5 10h17"/>`),
	"folderCheck":  icon(`<path d="M3.5 7.5a2 2 0 0 1 2-2h4l2 2h7a2 2 0 0 1 2 2v8.5a2 2 0 0 1-2 2h-13a2 2 0 0 1-2-2z"/><path d="M3.5 10h17"/><path d="m9 15 2 2 4-4"/>`),
	"device":       navIcons["devices"],
	"shield":       navIcons["security"],
	"logout":       icon(`<path d="M10 5H6.5A2.5 2.5 0 0 0 4 7.5v9A2.5 2.5 0 0 0 6.5 19H10"/><path d="M14 8l4 4-4 4M18 12H9"/>`),
	"warning":      icon(`<path d="M10.2 4.3 2.9 17a2 2 0 0 0 1.7 3h14.8a2 2 0 0 0 1.7-3L13.8 4.3a2.1 2.1 0 0 0-3.6 0Z"/><path d="M12 9v4M12 17h.01"/>`),
	"check":        icon(`<path d="m5 12.5 4.2 4.2L19 7"/>`),
	"arrowRight":   icon(`<path d="M5 12h14M14 7l5 5-5 5"/>`),
	"copy":         icon(`<rect x="8" y="8" width="11" height="11" rx="2.5"/><path d="M16 8V6.5A2.5 2.5 0 0 0 13.5 4h-7A2.5 2.5 0 0 0 4 6.5v7A2.5 2.5 0 0 0 6.5 16H8"/>`),
	"crown":        icon(`<path d="m4 8 4 3 4-6 4 6 4-3-2 10H6z"/><path d="M7 21h10"/>`),
	"medal":        icon(`<circle cx="12" cy="9" r="5"/><path d="m8.5 13-2 7 5.5-2 5.5 2-2-7"/>`),
	"sparkles":     icon(`<path d="m12 3 1.1 3.1L16 7.2l-2.9 1.1L12 11.5l-1.1-3.2L8 7.2l2.9-1.1zM18.5 13l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8zM5.5 12l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8z"/>`),
	"menu":         icon(`<path d="M5 7h14M5 12h14M5 17h14"/>`),
	"more":         icon(`<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>`),
	"chevronRight": icon(`<path d="m9 5 7 7-7 7"/>`),
	"userPlus":     navIcons["invite"],
	"chatgpt":      icon(`<path d="M12 3.8a4.2 4.2 0 0 1 4 2.9l2.5 1.4a4.2 4.2 0 0 1 0 7.3L16 16.9a4.2 4.2 0 0 1-4 2.9 4.2 4.2 0 0 1-4-2.9l-2.5-1.5a4.2 4.2 0 0 1 0-7.3L8 6.7a4.2 4.2 0 0 1 4-2.9Z"/><path d="m8 6.7 4 2.3 4-2.3M5.5 8.1 9.5 10.4v4.7M5.5 15.4l4-2.3 4 2.3 4-2.3M14.5 9v4.7L18.5 16"/>`),
}
