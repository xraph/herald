package api

import (
	"context"

	"github.com/xraph/forge"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/inbox"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/store"
	"github.com/xraph/herald/template"
)

// Every by-ID route loads the row and compares its app to app_id. A row from
// another app is a 404, exactly like a row that doesn't exist, so a response
// never confirms that an ID exists elsewhere. An absent app_id means the ""
// app, the same exact match every list route already uses.

func parseProviderID(raw string) (id.ProviderID, error) {
	pid, err := id.ParseProviderID(raw)
	if err != nil {
		return id.Nil, forge.BadRequest("invalid provider ID")
	}
	return pid, nil
}

func (a *ForgeAPI) ownedTemplate(ctx context.Context, appID, raw string) (*template.Template, error) {
	tid, err := id.ParseTemplateID(raw)
	if err != nil {
		return nil, forge.BadRequest("invalid template ID")
	}
	t, err := a.store.GetTemplate(ctx, tid)
	if err != nil {
		return nil, mapError(err)
	}
	if t.AppID != appID {
		return nil, mapError(store.ErrTemplateNotFound)
	}
	return t, nil
}

func (a *ForgeAPI) ownedVersion(ctx context.Context, appID, templateRaw, versionRaw string) (*template.Version, error) {
	t, err := a.ownedTemplate(ctx, appID, templateRaw)
	if err != nil {
		return nil, err
	}
	vid, err := id.ParseTemplateVersionID(versionRaw)
	if err != nil {
		return nil, forge.BadRequest("invalid version ID")
	}
	v, err := a.store.GetVersion(ctx, vid)
	if err != nil {
		return nil, mapError(err)
	}
	if v.TemplateID.String() != t.ID.String() {
		return nil, mapError(store.ErrVersionNotFound)
	}
	return v, nil
}

func (a *ForgeAPI) ownedMessage(ctx context.Context, appID, raw string) (*message.Message, error) {
	mid, err := id.ParseMessageID(raw)
	if err != nil {
		return nil, forge.BadRequest("invalid message ID")
	}
	m, err := a.store.GetMessage(ctx, mid)
	if err != nil {
		return nil, mapError(err)
	}
	if m.AppID != appID {
		return nil, mapError(store.ErrMessageNotFound)
	}
	return m, nil
}

func (a *ForgeAPI) ownedNotification(ctx context.Context, appID, raw string) (*inbox.Notification, error) {
	nid, err := id.ParseInboxID(raw)
	if err != nil {
		return nil, forge.BadRequest("invalid notification ID")
	}
	n, err := a.store.GetNotification(ctx, nid)
	if err != nil {
		return nil, mapError(err)
	}
	if n.AppID != appID {
		return nil, mapError(store.ErrNotificationNotFound)
	}
	return n, nil
}
