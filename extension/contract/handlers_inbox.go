package contract

import (
	"context"
	"strings"
	"time"

	"github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"

	"github.com/xraph/herald/id"
	"github.com/xraph/herald/inbox"
)

func registerInbox(d *dispatcher.Dispatcher, deps Deps) error {
	if err := query(d, "inbox.list", inboxListHandler(deps)); err != nil {
		return err
	}
	if err := command(d, "inbox.markRead", inboxMarkReadHandler(deps)); err != nil {
		return err
	}
	if err := command(d, "inbox.markAllRead", inboxMarkAllReadHandler(deps)); err != nil {
		return err
	}
	return command(d, "inbox.delete", inboxDeleteHandler(deps))
}

// NotificationWire is one in-app notification.
type NotificationWire struct {
	ID        string            `json:"id"`
	UserID    string            `json:"userId"`
	Type      string            `json:"type"`
	Title     string            `json:"title"`
	Body      string            `json:"body,omitempty"`
	ActionURL string            `json:"actionUrl,omitempty"`
	ImageURL  string            `json:"imageUrl,omitempty"`
	Read      bool              `json:"read"`
	ReadAt    *time.Time        `json:"readAt,omitempty"`
	Metadata  map[string]string `json:"metadata"`
	ExpiresAt *time.Time        `json:"expiresAt,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
}

func projectNotification(n *inbox.Notification) NotificationWire {
	md := n.Metadata
	if md == nil {
		md = map[string]string{}
	}
	return NotificationWire{
		ID: n.ID.String(), UserID: n.UserID, Type: n.Type, Title: n.Title, Body: n.Body,
		ActionURL: n.ActionURL, ImageURL: n.ImageURL, Read: n.Read, ReadAt: n.ReadAt,
		Metadata: md, ExpiresAt: n.ExpiresAt, CreatedAt: n.CreatedAt,
	}
}

func requireUser(raw string) (string, error) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", badRequest("userId is required")
	}
	return u, nil
}

type inboxListRequest struct {
	UserID string `json:"userId"`
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}

type inboxListResponse struct {
	Notifications []NotificationWire `json:"notifications"`
	Unread        int                `json:"unread"`
	NextCursor    string             `json:"nextCursor,omitempty"`
}

// inboxListHandler pages one user's in-app notifications in this app, with
// the user's unread count across all pages.
func inboxListHandler(deps Deps) func(context.Context, inboxListRequest, contract.Principal) (inboxListResponse, error) {
	return func(ctx context.Context, in inboxListRequest, p contract.Principal) (inboxListResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return inboxListResponse{}, err
		}
		userID, err := requireUser(in.UserID)
		if err != nil {
			return inboxListResponse{}, err
		}
		offset, err := decodeCursor(in.Cursor)
		if err != nil {
			return inboxListResponse{}, err
		}
		limit := pageLimit(in.Limit, defaultPageLimit, maxPageLimit)
		rows, err := deps.Herald.Store().ListNotifications(ctx, appID, userID, limit+1, offset)
		if err != nil {
			return inboxListResponse{}, deps.mapError("inbox.list", err)
		}
		unread, err := deps.Herald.Store().UnreadCount(ctx, appID, userID)
		if err != nil {
			return inboxListResponse{}, deps.mapError("inbox.list", err)
		}
		out := inboxListResponse{Notifications: make([]NotificationWire, 0, limit), Unread: unread, NextCursor: nextCursor(offset, limit, len(rows))}
		for i, n := range rows {
			if i == limit {
				break
			}
			out.Notifications = append(out.Notifications, projectNotification(n))
		}
		return out, nil
	}
}

type inboxIDRequest struct {
	ID string `json:"id"`
}

type inboxUserRequest struct {
	UserID string `json:"userId"`
}

type inboxOKResponse struct {
	OK bool   `json:"ok"`
	ID string `json:"id,omitempty"`
}

// ownedNotification loads a notification and answers NOT_FOUND for one in
// another app, the same answer as for one that doesn't exist.
func ownedNotification(ctx context.Context, deps Deps, appID, raw, intent string) (*inbox.Notification, error) {
	nid, err := id.ParseInboxID(strings.TrimSpace(raw))
	if err != nil {
		return nil, badRequest("id is not a notification id")
	}
	n, err := deps.Herald.Store().GetNotification(ctx, nid)
	if err != nil {
		return nil, deps.mapError(intent, err)
	}
	if n.AppID != appID {
		return nil, notFound("notification not found")
	}
	return n, nil
}

func inboxMarkReadHandler(deps Deps) func(context.Context, inboxIDRequest, contract.Principal) (inboxOKResponse, error) {
	return func(ctx context.Context, in inboxIDRequest, p contract.Principal) (inboxOKResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return inboxOKResponse{}, err
		}
		n, err := ownedNotification(ctx, deps, appID, in.ID, "inbox.markRead")
		if err != nil {
			return inboxOKResponse{}, err
		}
		if n.Read {
			// Already read: keep the original ReadAt and don't audit a no-op.
			return inboxOKResponse{OK: true, ID: n.ID.String()}, nil
		}
		if err := deps.Herald.Store().MarkRead(ctx, n.ID); err != nil {
			return inboxOKResponse{}, deps.mapError("inbox.markRead", err)
		}
		audit(ctx, deps, p, appID, "inbox.markRead", "notification", n.ID.String(), map[string]string{"user_id": n.UserID})
		return inboxOKResponse{OK: true, ID: n.ID.String()}, nil
	}
}

func inboxMarkAllReadHandler(deps Deps) func(context.Context, inboxUserRequest, contract.Principal) (inboxOKResponse, error) {
	return func(ctx context.Context, in inboxUserRequest, p contract.Principal) (inboxOKResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return inboxOKResponse{}, err
		}
		userID, err := requireUser(in.UserID)
		if err != nil {
			return inboxOKResponse{}, err
		}
		if err := deps.Herald.Store().MarkAllRead(ctx, appID, userID); err != nil {
			return inboxOKResponse{}, deps.mapError("inbox.markAllRead", err)
		}
		audit(ctx, deps, p, appID, "inbox.markAllRead", "inbox", userID, nil)
		return inboxOKResponse{OK: true}, nil
	}
}

func inboxDeleteHandler(deps Deps) func(context.Context, inboxIDRequest, contract.Principal) (deleteResponse, error) {
	return func(ctx context.Context, in inboxIDRequest, p contract.Principal) (deleteResponse, error) {
		appID, err := resolveApp(p, deps)
		if err != nil {
			return deleteResponse{}, err
		}
		n, err := ownedNotification(ctx, deps, appID, in.ID, "inbox.delete")
		if err != nil {
			return deleteResponse{}, err
		}
		if err := deps.Herald.Store().DeleteNotification(ctx, n.ID); err != nil {
			return deleteResponse{}, deps.mapError("inbox.delete", err)
		}
		audit(ctx, deps, p, appID, "inbox.delete", "notification", n.ID.String(), map[string]string{"user_id": n.UserID})
		return deleteResponse{OK: true, ID: n.ID.String()}, nil
	}
}
