package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
)

type OrganizationRule struct {
	OwnerUserID    string         `json:"ownerUserId"`
	OrganizationID string         `json:"organizationId"`
	RuleID         string         `json:"ruleId"`
	Text           string         `json:"text"`
	ApplyTo        []string       `json:"applyTo,omitempty"`
	Required       bool           `json:"required"`
	Status         string         `json:"status"`
	Revision       int64          `json:"revision"`
	CreatedAt      int64          `json:"createdAt"`
	UpdatedAt      int64          `json:"updatedAt"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type OrganizationRuleInput struct {
	OwnerUserID    string
	OrganizationID string
	RuleID         string
	Text           string
	ApplyTo        []string
	Required       bool
	Metadata       map[string]any
}

const organizationRulesForProjectSQL = `
SELECT r.owner_user_id,r.organization_id,r.rule_id,r.rule_text,r.apply_to,r.required,r.status,r.revision,r.created_at,r.updated_at,r.metadata
FROM codelocal_project_organizations p
JOIN codelocal_organization_rules r
 ON r.owner_user_id=p.owner_user_id AND r.organization_id=p.organization_id
WHERE p.owner_user_id=$1 AND p.project_id=$2 AND r.status='active'
ORDER BY r.organization_id ASC,r.rule_id ASC`

func normalizeOrganizationRuleInput(input OrganizationRuleInput) (OrganizationRuleInput, error) {
	input.OwnerUserID = strings.TrimSpace(input.OwnerUserID)
	input.OrganizationID = strings.TrimSpace(input.OrganizationID)
	input.RuleID = strings.TrimSpace(input.RuleID)
	input.Text = longmemory.SanitizeText(input.Text, 1600)
	input.ApplyTo = longmemory.SanitizeList(input.ApplyTo, 64)
	if input.OwnerUserID == "" || input.OrganizationID == "" || input.Text == "" {
		return OrganizationRuleInput{}, ErrKnowledgeInvalidScope
	}
	if input.RuleID == "" {
		input.RuleID = knowledgeStableID("org_rule_", input.OwnerUserID, input.OrganizationID, input.Text, strings.Join(input.ApplyTo, "\x00"))
	}
	return input, nil
}

func (s *Store) EnsureOrganization(ctx context.Context, ownerUserID, organizationID, name string) error {
	ownerUserID = strings.TrimSpace(ownerUserID)
	organizationID = strings.TrimSpace(organizationID)
	name = longmemory.SanitizeText(name, 160)
	if s == nil || s.DB == nil || ownerUserID == "" || organizationID == "" || name == "" {
		return ErrKnowledgeInvalidScope
	}
	now := time.Now().UnixMilli()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_organizations(owner_user_id,organization_id,name,created_at,last_seen_at)
VALUES($1,$2,$3,$4,$4)
ON CONFLICT(owner_user_id,organization_id) DO UPDATE SET name=EXCLUDED.name,last_seen_at=EXCLUDED.last_seen_at`, ownerUserID, organizationID, name, now); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_organization_members(owner_user_id,organization_id,member_user_id,role,status,added_at,updated_at)
VALUES($1,$2,$1,'owner','active',$3,$3)
ON CONFLICT(owner_user_id,organization_id,member_user_id) DO UPDATE SET role='owner',status='active',updated_at=EXCLUDED.updated_at`, ownerUserID, organizationID, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) BindProjectOrganization(ctx context.Context, ownerUserID, projectID, organizationID string) error {
	ownerUserID = strings.TrimSpace(ownerUserID)
	projectID = strings.TrimSpace(projectID)
	organizationID = strings.TrimSpace(organizationID)
	if s == nil || s.DB == nil || ownerUserID == "" || projectID == "" || organizationID == "" {
		return ErrKnowledgeInvalidScope
	}
	_, err := s.DB.Exec(ctx, `
INSERT INTO codelocal_project_organizations(owner_user_id,project_id,organization_id,created_at)
VALUES($1,$2,$3,$4)
ON CONFLICT(owner_user_id,project_id,organization_id) DO NOTHING`, ownerUserID, projectID, organizationID, time.Now().UnixMilli())
	return err
}

func (s *Store) UpsertOrganizationRule(ctx context.Context, input OrganizationRuleInput) (OrganizationRule, error) {
	if s == nil || s.DB == nil {
		return OrganizationRule{}, errors.New("organization rule store unavailable")
	}
	normalized, err := normalizeOrganizationRuleInput(input)
	if err != nil {
		return OrganizationRule{}, err
	}
	now := time.Now().UnixMilli()
	var rule OrganizationRule
	var applyToRaw, metadataRaw []byte
	err = s.DB.QueryRow(ctx, `
INSERT INTO codelocal_organization_rules(owner_user_id,organization_id,rule_id,rule_text,apply_to,required,status,revision,created_at,updated_at,metadata)
VALUES($1,$2,$3,$4,$5::jsonb,$6,'active',1,$7,$7,$8)
ON CONFLICT(owner_user_id,organization_id,rule_id) DO UPDATE SET
 rule_text=EXCLUDED.rule_text,apply_to=EXCLUDED.apply_to,required=EXCLUDED.required,status='active',revision=codelocal_organization_rules.revision+1,updated_at=EXCLUDED.updated_at,metadata=EXCLUDED.metadata
RETURNING owner_user_id,organization_id,rule_id,rule_text,apply_to,required,status,revision,created_at,updated_at,metadata`,
		normalized.OwnerUserID, normalized.OrganizationID, normalized.RuleID, normalized.Text, experienceJSON(normalized.ApplyTo), normalized.Required, now, knowledgeMetadata(normalized.Metadata)).Scan(
		&rule.OwnerUserID, &rule.OrganizationID, &rule.RuleID, &rule.Text, &applyToRaw, &rule.Required, &rule.Status, &rule.Revision, &rule.CreatedAt, &rule.UpdatedAt, &metadataRaw)
	if err != nil {
		return OrganizationRule{}, err
	}
	_ = json.Unmarshal(applyToRaw, &rule.ApplyTo)
	_ = json.Unmarshal(metadataRaw, &rule.Metadata)
	return rule, nil
}

func (s *Store) ListOrganizationRulesForProject(ctx context.Context, ownerUserID, projectID string) ([]OrganizationRule, error) {
	ownerUserID = strings.TrimSpace(ownerUserID)
	projectID = strings.TrimSpace(projectID)
	if s == nil || s.DB == nil || ownerUserID == "" || projectID == "" {
		return nil, ErrKnowledgeInvalidScope
	}
	rows, err := s.DB.Query(ctx, organizationRulesForProjectSQL, ownerUserID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OrganizationRule{}
	for rows.Next() {
		var item OrganizationRule
		var applyToRaw, metadataRaw []byte
		if err := rows.Scan(&item.OwnerUserID, &item.OrganizationID, &item.RuleID, &item.Text, &applyToRaw, &item.Required, &item.Status, &item.Revision, &item.CreatedAt, &item.UpdatedAt, &metadataRaw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(applyToRaw, &item.ApplyTo)
		_ = json.Unmarshal(metadataRaw, &item.Metadata)
		out = append(out, item)
	}
	return out, rows.Err()
}
