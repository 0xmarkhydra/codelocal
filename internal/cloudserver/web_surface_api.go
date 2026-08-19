package cloudserver

import (
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

type webInviteMemberDTO struct {
	EmailMasked string `json:"emailMasked"`
	Initial     string `json:"initial"`
	JoinedAt    int64  `json:"joinedAt"`
	Status      string `json:"status"`
}

type webInviteDTO struct {
	ReferralCode string               `json:"referralCode"`
	InvitedBy    string               `json:"invitedBy"`
	InviteLink   string               `json:"inviteLink"`
	DirectCount  int                  `json:"directCount"`
	ActiveCount  int                  `json:"activeCount"`
	Members      []webInviteMemberDTO `json:"members"`
}

type webLeaderboardEntryDTO struct {
	Rank        int    `json:"rank"`
	EmailMasked string `json:"emailMasked"`
	Initial     string `json:"initial"`
	Calls       int64  `json:"calls"`
	Tokens      int64  `json:"tokens"`
	IsCurrent   bool   `json:"isCurrent"`
}

type webLeaderboardDTO struct {
	WindowDays    int                      `json:"windowDays"`
	CurrentRank   int                      `json:"currentRank"`
	CurrentTokens int64                    `json:"currentTokens"`
	Entries       []webLeaderboardEntryDTO `json:"entries"`
}

type webAdminUserDTO struct {
	ID               string `json:"id"`
	Email            string `json:"email"`
	ReferralCode     string `json:"referralCode"`
	ReferredByCode   string `json:"referredByCode"`
	CreatedAt        int64  `json:"createdAt"`
	InviteCount      int64  `json:"inviteCount"`
	LastDeviceSeenAt int64  `json:"lastDeviceSeenAt"`
	LastMCPUsedAt    int64  `json:"lastMcpUsedAt"`
	RuntimeActive    bool   `json:"runtimeActive"`
	MCPActive        bool   `json:"mcpActive"`
}

type webAdminDTO struct {
	TotalUsers    int               `json:"totalUsers"`
	ActiveUsers   int               `json:"activeUsers"`
	RuntimeOnline int               `json:"runtimeOnline"`
	UsingMCPNow   int               `json:"usingMcpNow"`
	Users         []webAdminUserDTO `json:"users"`
}

type webPairApproveDTO struct {
	PairingID  string `json:"pairingId"`
	Code       string `json:"code"`
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	ExpiresAt  int64  `json:"expiresAt"`
}

func (s *Server) authCSRFResourceAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	webutil.JSON(w, http.StatusOK, map[string]string{"csrf": s.WebAuth.EnsureCSRF(w, r)})
}

func (s *Server) inviteResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	users, err := s.Store.ListInvitedUsers(r.Context(), identity.User.ReferralCode)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "invite_unavailable"})
		return
	}
	ids := make([]string, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
	}
	runtimeActive, _ := s.Activation.UserOnlineMap(r.Context(), ids)
	mcpActive, _ := s.Store.UserMCPActiveMap(r.Context(), ids)
	members := make([]webInviteMemberDTO, 0, len(users))
	activeCount := 0
	for _, user := range users {
		status := "offline"
		if mcpActive[user.ID] {
			status = "mcp_active"
			activeCount++
		} else if runtimeActive[user.ID] {
			status = "runtime_online"
			activeCount++
		}
		members = append(members, webInviteMemberDTO{
			EmailMasked: maskLeaderboardEmail(user.Email),
			Initial:     mainLeaderboardInitial(user.Email),
			JoinedAt:    user.CreatedAt,
			Status:      status,
		})
	}
	invitedBy := identity.User.ReferredByCode
	if invitedBy == "" {
		invitedBy = "Root account"
	}
	inviteLink := strings.TrimRight(s.WebAuth.PublicBaseURL, "/") + "/register?ref=" + url.QueryEscape(identity.User.ReferralCode)
	webutil.JSON(w, http.StatusOK, webInviteDTO{
		ReferralCode: identity.User.ReferralCode,
		InvitedBy:    invitedBy,
		InviteLink:   inviteLink,
		DirectCount:  len(members),
		ActiveCount:  activeCount,
		Members:      members,
	})
}

func (s *Server) leaderboardResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	now := time.Now().UnixMilli()
	since := time.Now().Add(-30 * 24 * time.Hour).UnixMilli()
	users, err := s.Store.ListUsageLeaderboardUsers(r.Context(), since)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "leaderboard_unavailable"})
		return
	}
	start := mainUsageDayStart(since)
	step := int64(24 * time.Hour / time.Millisecond)
	entries := make([]mainUsageLeaderboardEntry, 0, len(users))
	commands := make([]mainUsageLeaderboardCommand, 0, len(users)*31)
	pipe := s.Store.Redis.Pipeline()
	for _, user := range users {
		entries = append(entries, mainUsageLeaderboardEntry{Email: user.Email})
		entry := &entries[len(entries)-1]
		for at := start; at <= now; at += step {
			commands = append(commands, mainUsageLeaderboardCommand{entry: entry, cmd: pipe.HGetAll(r.Context(), mainUsageDayKey(user.ID, at))})
		}
	}
	if len(commands) > 0 {
		if _, err := pipe.Exec(r.Context()); err != nil {
			webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "leaderboard_unavailable"})
			return
		}
	}
	for _, pending := range commands {
		values := pending.cmd.Val()
		pending.entry.Calls += mainUsageValue(values, "calls")
		pending.entry.Tokens += mainUsageValue(values, "input_tokens") + mainUsageValue(values, "output_tokens")
	}
	active := entries[:0]
	for _, entry := range entries {
		if entry.Calls > 0 || entry.Tokens > 0 {
			active = append(active, entry)
		}
	}
	entries = active
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Tokens != entries[j].Tokens {
			return entries[i].Tokens > entries[j].Tokens
		}
		if entries[i].Calls != entries[j].Calls {
			return entries[i].Calls > entries[j].Calls
		}
		return strings.ToLower(entries[i].Email) < strings.ToLower(entries[j].Email)
	})
	response := webLeaderboardDTO{WindowDays: 30, Entries: make([]webLeaderboardEntryDTO, 0, min(len(entries), 50))}
	for i, entry := range entries {
		isCurrent := strings.EqualFold(entry.Email, identity.User.Email)
		if isCurrent {
			response.CurrentRank = i + 1
			response.CurrentTokens = entry.Tokens
		}
		if i >= 50 {
			continue
		}
		response.Entries = append(response.Entries, webLeaderboardEntryDTO{
			Rank: i + 1, EmailMasked: maskLeaderboardEmail(entry.Email), Initial: mainLeaderboardInitial(entry.Email),
			Calls: entry.Calls, Tokens: entry.Tokens, IsCurrent: isCurrent,
		})
	}
	webutil.JSON(w, http.StatusOK, response)
}

func (s *Server) adminResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	if !cloud.IsAdminEmail(identity.User.Email) {
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	users, err := s.Store.ListAdminUsers(r.Context())
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "admin_unavailable"})
		return
	}
	ids := make([]string, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
	}
	runtimeActive, _ := s.Activation.UserOnlineMap(r.Context(), ids)
	mcpActive, _ := s.Store.UserMCPActiveMap(r.Context(), ids)
	response := webAdminDTO{Users: make([]webAdminUserDTO, 0, len(users)), TotalUsers: len(users)}
	for _, user := range users {
		runtimeOnline := runtimeActive[user.ID]
		mcpNow := mcpActive[user.ID]
		if runtimeOnline {
			response.RuntimeOnline++
		}
		if mcpNow {
			response.UsingMCPNow++
		}
		if runtimeOnline || mcpNow {
			response.ActiveUsers++
		}
		response.Users = append(response.Users, webAdminUserDTO{
			ID: user.ID, Email: user.Email, ReferralCode: user.ReferralCode, ReferredByCode: user.ReferredByCode,
			CreatedAt: user.CreatedAt, InviteCount: user.InviteCount, LastDeviceSeenAt: user.LastDeviceSeenAt,
			LastMCPUsedAt: user.LastMCPUsedAt, RuntimeActive: runtimeOnline, MCPActive: mcpNow,
		})
	}
	webutil.JSON(w, http.StatusOK, response)
}

func (s *Server) pairApproveResourceAPI(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedAPIIdentity(w, r); !ok {
		return
	}
	pairingID := strings.TrimSpace(r.URL.Query().Get("pairingId"))
	pairing, _ := s.Store.GetPairing(r.Context(), pairingID)
	if pairing == nil || pairing.ExpiresAt <= time.Now().UnixMilli() || pairing.ClaimedAt != 0 {
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error": "pairing_unavailable"})
		return
	}
	webutil.JSON(w, http.StatusOK, webPairApproveDTO{
		PairingID: pairing.PairingID, Code: pairing.Code, DeviceID: pairing.DeviceID,
		DeviceName: pairing.DeviceName, ExpiresAt: pairing.ExpiresAt,
	})
}
