// Package api provides the Forge-integrated HTTP API for Herald.
package api

import (
	"net/http"
	"time"

	"github.com/xraph/forge"

	"github.com/xraph/herald"
	"github.com/xraph/herald/bridge"
	"github.com/xraph/herald/id"
	"github.com/xraph/herald/message"
	"github.com/xraph/herald/preference"
	"github.com/xraph/herald/provider"
	"github.com/xraph/herald/scope"
	"github.com/xraph/herald/store"
	"github.com/xraph/herald/template"
)

// ForgeAPI is the Forge-integrated HTTP API handler for Herald.
type ForgeAPI struct {
	store  store.Store
	herald *herald.Herald
	logger forge.Logger
}

// NewForgeAPI creates a new Forge API handler.
func NewForgeAPI(s store.Store, h *herald.Herald, logger forge.Logger) *ForgeAPI {
	return &ForgeAPI{
		store:  s,
		herald: h,
		logger: logger,
	}
}

// RegisterRoutes registers all Herald API routes with a Forge router.
func (a *ForgeAPI) RegisterRoutes(router forge.Router) {
	a.registerProviderRoutes(router)
	a.registerTemplateRoutes(router)
	a.registerSendRoutes(router)
	a.registerMessageRoutes(router)
	a.registerInboxRoutes(router)
	a.registerPreferenceRoutes(router)
	a.registerConfigRoutes(router)
}

// ─── Request Types ──────────────────────────────

// Provider requests
type CreateProviderRequest struct {
	AppID       string            `description:"Application ID"     json:"app_id"`
	Name        string            `description:"Provider name"      json:"name"`
	Channel     string            `description:"Channel type"       json:"channel"`
	Driver      string            `description:"Driver name"        json:"driver"`
	Credentials map[string]string `description:"Driver credentials" json:"credentials"`
	Settings    map[string]string `description:"Driver settings"    json:"settings"`
	Priority    int               `description:"Priority order"     json:"priority"`
	Enabled     bool              `description:"Is enabled"         json:"enabled"`
}

type ListProvidersRequest struct {
	AppID   string `description:"Filter by app"     query:"app_id"`
	Channel string `description:"Filter by channel" query:"channel"`
}

type GetProviderRequest struct {
	ID string `description:"Provider ID" path:"id"`
}

type UpdateProviderRequest struct {
	ID          string            `description:"Provider ID"        path:"id"`
	Name        string            `description:"Provider name"      json:"name,omitempty"`
	Channel     string            `description:"Channel type"       json:"channel,omitempty"`
	Driver      string            `description:"Driver name"        json:"driver,omitempty"`
	Credentials map[string]string `description:"Driver credentials" json:"credentials,omitempty"`
	Settings    map[string]string `description:"Driver settings"    json:"settings,omitempty"`
	Priority    int               `description:"Priority order"     json:"priority,omitempty"`
	Enabled     bool              `description:"Is enabled"         json:"enabled,omitempty"`
}

type DeleteProviderRequest struct {
	ID string `description:"Provider ID" path:"id"`
}

// Template requests
type CreateTemplateRequest struct {
	AppID     string              `description:"Application ID"      json:"app_id"`
	Slug      string              `description:"Template slug"       json:"slug"`
	Name      string              `description:"Template name"       json:"name"`
	Channel   string              `description:"Channel type"        json:"channel"`
	Category  string              `description:"Template category"   json:"category"`
	Variables []template.Variable `description:"Template variables"  json:"variables"`
	Enabled   bool                `description:"Is enabled"          json:"enabled"`
}

type ListTemplatesRequest struct {
	AppID   string `description:"Filter by app"     query:"app_id"`
	Channel string `description:"Filter by channel" query:"channel"`
}

type GetTemplateRequest struct {
	ID string `description:"Template ID" path:"id"`
}

type UpdateTemplateRequest struct {
	ID        string              `description:"Template ID"         path:"id"`
	Slug      string              `description:"Template slug"       json:"slug,omitempty"`
	Name      string              `description:"Template name"       json:"name,omitempty"`
	Channel   string              `description:"Channel type"        json:"channel,omitempty"`
	Category  string              `description:"Template category"   json:"category,omitempty"`
	Variables []template.Variable `description:"Template variables"  json:"variables,omitempty"`
	Enabled   bool                `description:"Is enabled"          json:"enabled,omitempty"`
}

type DeleteTemplateRequest struct {
	ID string `description:"Template ID" path:"id"`
}

type CreateVersionRequest struct {
	TemplateID string `description:"Template ID" path:"id"`
	Locale     string `description:"Locale code" json:"locale"`
	Subject    string `description:"Subject"     json:"subject"`
	HTML       string `description:"HTML body"   json:"html"`
	Text       string `description:"Text body"   json:"text"`
	Title      string `description:"Title"       json:"title"`
}

type ListVersionsRequest struct {
	TemplateID string `description:"Template ID" path:"id"`
}

type UpdateVersionRequest struct {
	TemplateID string `description:"Template ID"         path:"id"`
	VersionID  string `description:"Version ID"          path:"versionId"`
	Locale     string `description:"Locale code"         json:"locale,omitempty"`
	Subject    string `description:"Subject"             json:"subject,omitempty"`
	HTML       string `description:"HTML body"           json:"html,omitempty"`
	Text       string `description:"Text body"           json:"text,omitempty"`
	Title      string `description:"Title"               json:"title,omitempty"`
	Active     *bool  `description:"Is active"           json:"active,omitempty"`
}

type DeleteVersionRequest struct {
	TemplateID string `description:"Template ID" path:"id"`
	VersionID  string `description:"Version ID"  path:"versionId"`
}

// Message requests
type ListMessagesRequest struct {
	AppID   string `description:"Filter by app"     query:"app_id"`
	Channel string `description:"Filter by channel" query:"channel"`
	Status  string `description:"Filter by status"  query:"status"`
	Offset  int    `description:"Pagination offset" query:"offset"`
	Limit   int    `description:"Page size"         query:"limit"`
}

type GetMessageRequest struct {
	ID string `description:"Message ID" path:"id"`
}

// Inbox requests
type ListInboxRequest struct {
	AppID  string `description:"Application ID" query:"app_id"`
	UserID string `description:"User ID"        query:"user_id"`
	Offset int    `description:"Offset"         query:"offset"`
	Limit  int    `description:"Page size"      query:"limit"`
}

type UnreadCountRequest struct {
	AppID  string `description:"Application ID" query:"app_id"`
	UserID string `description:"User ID"        query:"user_id"`
}

type MarkReadRequest struct {
	ID string `description:"Notification ID" path:"id"`
}

type MarkAllReadRequest struct {
	AppID  string `description:"Application ID" query:"app_id"`
	UserID string `description:"User ID"        query:"user_id"`
}

type DeleteInboxRequest struct {
	ID string `description:"Notification ID" path:"id"`
}

// Preference requests
type GetPreferencesRequest struct {
	AppID  string `description:"Application ID" query:"app_id"`
	UserID string `description:"User ID"        query:"user_id"`
}

type UpdatePreferencesRequest struct {
	AppID     string                                  `description:"Application ID" json:"app_id"`
	UserID    string                                  `description:"User ID"        json:"user_id"`
	Overrides map[string]preference.ChannelPreference `description:"Overrides"    json:"overrides"`
}

// Config requests
type GetConfigRequest struct {
	AppID string `description:"Application ID" query:"app_id"`
}

type SetAppConfigRequest struct {
	AppID           string `description:"Application ID"    json:"app_id"`
	EmailProviderID string `description:"Email provider ID" json:"email_provider_id,omitempty"`
	SMSProviderID   string `description:"SMS provider ID"   json:"sms_provider_id,omitempty"`
	PushProviderID  string `description:"Push provider ID"  json:"push_provider_id,omitempty"`
	FromEmail       string `description:"Sender email"      json:"from_email,omitempty"`
	FromName        string `description:"Sender name"       json:"from_name,omitempty"`
	FromPhone       string `description:"Sender phone"      json:"from_phone,omitempty"`
	DefaultLocale   string `description:"Default locale"    json:"default_locale,omitempty"`
}

type SetOrgConfigRequest struct {
	OrgID           string `description:"Organization ID"   path:"orgId"`
	AppID           string `description:"Application ID"    json:"app_id"`
	EmailProviderID string `description:"Email provider ID" json:"email_provider_id,omitempty"`
	SMSProviderID   string `description:"SMS provider ID"   json:"sms_provider_id,omitempty"`
	PushProviderID  string `description:"Push provider ID"  json:"push_provider_id,omitempty"`
	FromEmail       string `description:"Sender email"      json:"from_email,omitempty"`
	FromName        string `description:"Sender name"       json:"from_name,omitempty"`
	FromPhone       string `description:"Sender phone"      json:"from_phone,omitempty"`
	DefaultLocale   string `description:"Default locale"    json:"default_locale,omitempty"`
}

type SetUserConfigRequest struct {
	UserID          string `description:"User ID"           path:"userId"`
	AppID           string `description:"Application ID"    json:"app_id"`
	EmailProviderID string `description:"Email provider ID" json:"email_provider_id,omitempty"`
	SMSProviderID   string `description:"SMS provider ID"   json:"sms_provider_id,omitempty"`
	PushProviderID  string `description:"Push provider ID"  json:"push_provider_id,omitempty"`
	FromEmail       string `description:"Sender email"      json:"from_email,omitempty"`
	FromName        string `description:"Sender name"       json:"from_name,omitempty"`
	FromPhone       string `description:"Sender phone"      json:"from_phone,omitempty"`
	DefaultLocale   string `description:"Default locale"    json:"default_locale,omitempty"`
}

type DeleteOrgConfigRequest struct {
	OrgID string `description:"Organization ID"   path:"orgId"`
	AppID string `description:"Application ID"    query:"app_id"`
}

type DeleteUserConfigRequest struct {
	UserID string `description:"User ID"           path:"userId"`
	AppID  string `description:"Application ID"    query:"app_id"`
}

// Response types
type UnreadCountResponse struct {
	Count int `json:"count"`
}

// ─── Route Registration ──────────────────────────────

func (a *ForgeAPI) registerProviderRoutes(router forge.Router) {
	g := router.Group("/providers", forge.WithGroupTags("providers"))

	if err := g.POST("", a.createProvider,
		forge.WithSummary("Create provider"),
		forge.WithDescription("Creates a new notification provider."),
		forge.WithOperationID("createProvider"),
		forge.WithRequestSchema(CreateProviderRequest{}),
		forge.WithCreatedResponse(provider.Provider{}),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register createProvider route", forge.Error(err))
	}

	if err := g.GET("", a.listProviders,
		forge.WithSummary("List providers"),
		forge.WithDescription("Returns a list of notification providers."),
		forge.WithOperationID("listProviders"),
		forge.WithRequestSchema(ListProvidersRequest{}),
		forge.WithListResponse(provider.Provider{}, http.StatusOK),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register listProviders route", forge.Error(err))
	}

	if err := g.GET("/:id", a.getProvider,
		forge.WithSummary("Get provider"),
		forge.WithDescription("Returns details of a specific provider."),
		forge.WithOperationID("getProvider"),
		forge.WithResponseSchema(http.StatusOK, "Provider details", provider.Provider{}),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register getProvider route", forge.Error(err))
	}

	if err := g.PUT("/:id", a.updateProvider,
		forge.WithSummary("Update provider"),
		forge.WithDescription("Updates an existing provider."),
		forge.WithOperationID("updateProvider"),
		forge.WithRequestSchema(UpdateProviderRequest{}),
		forge.WithResponseSchema(http.StatusOK, "Updated provider", provider.Provider{}),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register updateProvider route", forge.Error(err))
	}

	if err := g.DELETE("/:id", a.deleteProvider,
		forge.WithSummary("Delete provider"),
		forge.WithDescription("Deletes a provider."),
		forge.WithOperationID("deleteProvider"),
		forge.WithNoContentResponse(),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register deleteProvider route", forge.Error(err))
	}
}

func (a *ForgeAPI) registerTemplateRoutes(router forge.Router) {
	g := router.Group("/templates", forge.WithGroupTags("templates"))

	if err := g.POST("", a.createTemplate,
		forge.WithSummary("Create template"),
		forge.WithDescription("Creates a new notification template."),
		forge.WithOperationID("createTemplate"),
		forge.WithRequestSchema(CreateTemplateRequest{}),
		forge.WithCreatedResponse(template.Template{}),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register createTemplate route", forge.Error(err))
	}

	if err := g.GET("", a.listTemplates,
		forge.WithSummary("List templates"),
		forge.WithDescription("Returns a list of notification templates."),
		forge.WithOperationID("listTemplates"),
		forge.WithRequestSchema(ListTemplatesRequest{}),
		forge.WithListResponse(template.Template{}, http.StatusOK),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register listTemplates route", forge.Error(err))
	}

	if err := g.GET("/:id", a.getTemplate,
		forge.WithSummary("Get template"),
		forge.WithDescription("Returns details of a specific template with all versions."),
		forge.WithOperationID("getTemplate"),
		forge.WithResponseSchema(http.StatusOK, "Template details", template.Template{}),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register getTemplate route", forge.Error(err))
	}

	if err := g.PUT("/:id", a.updateTemplate,
		forge.WithSummary("Update template"),
		forge.WithDescription("Updates an existing template."),
		forge.WithOperationID("updateTemplate"),
		forge.WithRequestSchema(UpdateTemplateRequest{}),
		forge.WithResponseSchema(http.StatusOK, "Updated template", template.Template{}),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register updateTemplate route", forge.Error(err))
	}

	if err := g.DELETE("/:id", a.deleteTemplate,
		forge.WithSummary("Delete template"),
		forge.WithDescription("Deletes a template and all its versions."),
		forge.WithOperationID("deleteTemplate"),
		forge.WithNoContentResponse(),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register deleteTemplate route", forge.Error(err))
	}

	if err := g.POST("/:id/versions", a.createVersion,
		forge.WithSummary("Create version"),
		forge.WithDescription("Creates a new template version for a specific locale."),
		forge.WithOperationID("createVersion"),
		forge.WithRequestSchema(CreateVersionRequest{}),
		forge.WithCreatedResponse(template.Version{}),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register createVersion route", forge.Error(err))
	}

	if err := g.GET("/:id/versions", a.listVersions,
		forge.WithSummary("List versions"),
		forge.WithDescription("Returns all versions of a template."),
		forge.WithOperationID("listVersions"),
		forge.WithListResponse(template.Version{}, http.StatusOK),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register listVersions route", forge.Error(err))
	}

	if err := g.PUT("/:id/versions/:versionId", a.updateVersion,
		forge.WithSummary("Update version"),
		forge.WithDescription("Updates a template version."),
		forge.WithOperationID("updateVersion"),
		forge.WithRequestSchema(UpdateVersionRequest{}),
		forge.WithResponseSchema(http.StatusOK, "Updated version", template.Version{}),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register updateVersion route", forge.Error(err))
	}

	if err := g.DELETE("/:id/versions/:versionId", a.deleteVersion,
		forge.WithSummary("Delete version"),
		forge.WithDescription("Deletes a template version."),
		forge.WithOperationID("deleteVersion"),
		forge.WithNoContentResponse(),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register deleteVersion route", forge.Error(err))
	}
}

func (a *ForgeAPI) registerSendRoutes(router forge.Router) {
	g := router.Group("", forge.WithGroupTags("send"))

	if err := g.POST("/send", a.sendNotification,
		forge.WithSummary("Send notification"),
		forge.WithDescription("Sends a notification on a single channel."),
		forge.WithOperationID("sendNotification"),
		forge.WithRequestSchema(herald.SendRequest{}),
		forge.WithResponseSchema(http.StatusOK, "Send result", herald.SendResult{}),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register sendNotification route", forge.Error(err))
	}

	if err := g.POST("/notify", a.notifyMultiChannel,
		forge.WithSummary("Multi-channel notify"),
		forge.WithDescription("Sends a notification across multiple channels using a template."),
		forge.WithOperationID("notifyMultiChannel"),
		forge.WithRequestSchema(herald.NotifyRequest{}),
		forge.WithListResponse(herald.SendResult{}, http.StatusOK),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register notifyMultiChannel route", forge.Error(err))
	}
}

func (a *ForgeAPI) registerMessageRoutes(router forge.Router) {
	g := router.Group("/messages", forge.WithGroupTags("messages"))

	if err := g.GET("", a.listMessages,
		forge.WithSummary("List messages"),
		forge.WithDescription("Returns a list of sent messages (delivery log)."),
		forge.WithOperationID("listMessages"),
		forge.WithRequestSchema(ListMessagesRequest{}),
		forge.WithListResponse(message.Message{}, http.StatusOK),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register listMessages route", forge.Error(err))
	}

	if err := g.GET("/:id", a.getMessage,
		forge.WithSummary("Get message"),
		forge.WithDescription("Returns details of a specific message."),
		forge.WithOperationID("getMessage"),
		forge.WithResponseSchema(http.StatusOK, "Message details", message.Message{}),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register getMessage route", forge.Error(err))
	}
}

func (a *ForgeAPI) registerInboxRoutes(router forge.Router) {
	g := router.Group("/inbox", forge.WithGroupTags("inbox"))

	if err := g.GET("", a.listInbox,
		forge.WithSummary("List inbox"),
		forge.WithDescription("Returns a user's in-app notifications."),
		forge.WithOperationID("listInbox"),
		forge.WithRequestSchema(ListInboxRequest{}),
		forge.WithListResponse(struct{}{}, http.StatusOK),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register listInbox route", forge.Error(err))
	}

	if err := g.GET("/unread/count", a.unreadCount,
		forge.WithSummary("Unread count"),
		forge.WithDescription("Returns the unread notification count for a user."),
		forge.WithOperationID("unreadCount"),
		forge.WithRequestSchema(UnreadCountRequest{}),
		forge.WithResponseSchema(http.StatusOK, "Unread count", UnreadCountResponse{}),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register unreadCount route", forge.Error(err))
	}

	if err := g.PUT("/:id/read", a.markRead,
		forge.WithSummary("Mark read"),
		forge.WithDescription("Marks a notification as read."),
		forge.WithOperationID("markRead"),
		forge.WithNoContentResponse(),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register markRead route", forge.Error(err))
	}

	if err := g.PUT("/read-all", a.markAllRead,
		forge.WithSummary("Mark all read"),
		forge.WithDescription("Marks all notifications as read for a user."),
		forge.WithOperationID("markAllRead"),
		forge.WithRequestSchema(MarkAllReadRequest{}),
		forge.WithNoContentResponse(),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register markAllRead route", forge.Error(err))
	}

	if err := g.DELETE("/:id", a.deleteInboxItem,
		forge.WithSummary("Delete notification"),
		forge.WithDescription("Deletes an in-app notification."),
		forge.WithOperationID("deleteInboxItem"),
		forge.WithNoContentResponse(),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register deleteInboxItem route", forge.Error(err))
	}
}

func (a *ForgeAPI) registerPreferenceRoutes(router forge.Router) {
	g := router.Group("/preferences", forge.WithGroupTags("preferences"))

	if err := g.GET("", a.getPreferences,
		forge.WithSummary("Get preferences"),
		forge.WithDescription("Returns notification preferences for a user."),
		forge.WithOperationID("getPreferences"),
		forge.WithRequestSchema(GetPreferencesRequest{}),
		forge.WithResponseSchema(http.StatusOK, "User preferences", preference.Preference{}),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register getPreferences route", forge.Error(err))
	}

	if err := g.PUT("", a.updatePreferences,
		forge.WithSummary("Update preferences"),
		forge.WithDescription("Updates notification preferences for a user."),
		forge.WithOperationID("updatePreferences"),
		forge.WithRequestSchema(UpdatePreferencesRequest{}),
		forge.WithResponseSchema(http.StatusOK, "Updated preferences", preference.Preference{}),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register updatePreferences route", forge.Error(err))
	}
}

func (a *ForgeAPI) registerConfigRoutes(router forge.Router) {
	g := router.Group("/config", forge.WithGroupTags("config"))

	if err := g.GET("", a.getConfig,
		forge.WithSummary("Get config"),
		forge.WithDescription("Returns all scoped configurations for an app."),
		forge.WithOperationID("getConfig"),
		forge.WithRequestSchema(GetConfigRequest{}),
		forge.WithListResponse(scope.Config{}, http.StatusOK),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register getConfig route", forge.Error(err))
	}

	if err := g.PUT("/app", a.setAppConfig,
		forge.WithSummary("Set app config"),
		forge.WithDescription("Sets app-level notification configuration."),
		forge.WithOperationID("setAppConfig"),
		forge.WithRequestSchema(SetAppConfigRequest{}),
		forge.WithResponseSchema(http.StatusOK, "Updated config", scope.Config{}),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register setAppConfig route", forge.Error(err))
	}

	if err := g.PUT("/org/:orgId", a.setOrgConfig,
		forge.WithSummary("Set org config"),
		forge.WithDescription("Sets organization-level notification configuration."),
		forge.WithOperationID("setOrgConfig"),
		forge.WithRequestSchema(SetOrgConfigRequest{}),
		forge.WithResponseSchema(http.StatusOK, "Updated config", scope.Config{}),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register setOrgConfig route", forge.Error(err))
	}

	if err := g.PUT("/user/:userId", a.setUserConfig,
		forge.WithSummary("Set user config"),
		forge.WithDescription("Sets user-level notification configuration."),
		forge.WithOperationID("setUserConfig"),
		forge.WithRequestSchema(SetUserConfigRequest{}),
		forge.WithResponseSchema(http.StatusOK, "Updated config", scope.Config{}),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register setUserConfig route", forge.Error(err))
	}

	if err := g.DELETE("/org/:orgId", a.deleteOrgConfig,
		forge.WithSummary("Delete org config"),
		forge.WithDescription("Removes organization-level notification configuration."),
		forge.WithOperationID("deleteOrgConfig"),
		forge.WithNoContentResponse(),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register deleteOrgConfig route", forge.Error(err))
	}

	if err := g.DELETE("/user/:userId", a.deleteUserConfig,
		forge.WithSummary("Delete user config"),
		forge.WithDescription("Removes user-level notification configuration."),
		forge.WithOperationID("deleteUserConfig"),
		forge.WithNoContentResponse(),
		forge.WithErrorResponses(),
	); err != nil {
		a.logger.Error("failed to register deleteUserConfig route", forge.Error(err))
	}
}

// ─── Provider Handlers ──────────────────────────────

func (a *ForgeAPI) createProvider(ctx forge.Context, req *CreateProviderRequest) (*provider.Provider, error) {
	now := time.Now().UTC()
	p := &provider.Provider{
		ID:          id.NewProviderID(),
		AppID:       req.AppID,
		Name:        req.Name,
		Channel:     req.Channel,
		Driver:      req.Driver,
		Credentials: req.Credentials,
		Settings:    req.Settings,
		Priority:    req.Priority,
		Enabled:     req.Enabled,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := a.store.CreateProvider(ctx.Context(), p); err != nil {
		return nil, mapError(err)
	}
	a.herald.Audit(ctx.Context(), bridge.SeverityInfo, bridge.OutcomeSuccess, "provider.create", "provider", p.ID.String(), "", req.AppID, "provider", map[string]string{
		"name": req.Name, "channel": req.Channel, "driver": req.Driver,
	})
	if err := ctx.JSON(http.StatusCreated, p); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

func (a *ForgeAPI) listProviders(ctx forge.Context, req *ListProvidersRequest) (*struct{}, error) {
	var providers []*provider.Provider
	var err error
	if req.Channel != "" {
		providers, err = a.store.ListProviders(ctx.Context(), req.AppID, req.Channel)
	} else {
		providers, err = a.store.ListAllProviders(ctx.Context(), req.AppID)
	}
	if err != nil {
		return nil, mapError(err)
	}
	if err := ctx.JSON(http.StatusOK, providers); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

func (a *ForgeAPI) getProvider(ctx forge.Context, req *GetProviderRequest) (*provider.Provider, error) {
	pid, err := id.ParseProviderID(req.ID)
	if err != nil {
		return nil, forge.BadRequest("invalid provider ID")
	}
	p, err := a.store.GetProvider(ctx.Context(), pid)
	if err != nil {
		return nil, mapError(err)
	}
	return p, nil
}

func (a *ForgeAPI) updateProvider(ctx forge.Context, req *UpdateProviderRequest) (*provider.Provider, error) {
	pid, err := id.ParseProviderID(req.ID)
	if err != nil {
		return nil, forge.BadRequest("invalid provider ID")
	}
	existing, err := a.store.GetProvider(ctx.Context(), pid)
	if err != nil {
		return nil, mapError(err)
	}
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Channel != "" {
		existing.Channel = req.Channel
	}
	if req.Driver != "" {
		existing.Driver = req.Driver
	}
	if req.Credentials != nil {
		existing.Credentials = req.Credentials
	}
	if req.Settings != nil {
		existing.Settings = req.Settings
	}
	if req.Priority != 0 {
		existing.Priority = req.Priority
	}
	existing.Enabled = req.Enabled
	existing.UpdatedAt = time.Now().UTC()
	if err := a.store.UpdateProvider(ctx.Context(), existing); err != nil {
		return nil, mapError(err)
	}
	a.herald.Audit(ctx.Context(), bridge.SeverityInfo, bridge.OutcomeSuccess, "provider.update", "provider", existing.ID.String(), "", existing.AppID, "provider", map[string]string{
		"name": existing.Name, "channel": existing.Channel,
	})
	return existing, nil
}

func (a *ForgeAPI) deleteProvider(ctx forge.Context, req *DeleteProviderRequest) (*provider.Provider, error) {
	pid, err := id.ParseProviderID(req.ID)
	if err != nil {
		return nil, forge.BadRequest("invalid provider ID")
	}
	if err := a.store.DeleteProvider(ctx.Context(), pid); err != nil {
		return nil, mapError(err)
	}
	a.herald.Audit(ctx.Context(), bridge.SeverityWarning, bridge.OutcomeSuccess, "provider.delete", "provider", pid.String(), "", "", "provider", nil)
	if err := ctx.NoContent(http.StatusNoContent); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

// ─── Template Handlers ──────────────────────────────

func (a *ForgeAPI) createTemplate(ctx forge.Context, req *CreateTemplateRequest) (*template.Template, error) {
	now := time.Now().UTC()
	t := &template.Template{
		ID:        id.NewTemplateID(),
		AppID:     req.AppID,
		Slug:      req.Slug,
		Name:      req.Name,
		Channel:   req.Channel,
		Category:  req.Category,
		Variables: req.Variables,
		Enabled:   req.Enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := a.store.CreateTemplate(ctx.Context(), t); err != nil {
		return nil, mapError(err)
	}
	a.herald.Audit(ctx.Context(), bridge.SeverityInfo, bridge.OutcomeSuccess, "template.create", "template", t.ID.String(), "", req.AppID, "template", map[string]string{
		"slug": req.Slug, "channel": req.Channel, "category": req.Category,
	})
	if err := ctx.JSON(http.StatusCreated, t); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

func (a *ForgeAPI) listTemplates(ctx forge.Context, req *ListTemplatesRequest) (*struct{}, error) {
	var templates []*template.Template
	var err error
	if req.Channel != "" {
		templates, err = a.store.ListTemplatesByChannel(ctx.Context(), req.AppID, req.Channel)
	} else {
		templates, err = a.store.ListTemplates(ctx.Context(), req.AppID)
	}
	if err != nil {
		return nil, mapError(err)
	}
	if err := ctx.JSON(http.StatusOK, templates); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

func (a *ForgeAPI) getTemplate(ctx forge.Context, req *GetTemplateRequest) (*template.Template, error) {
	tid, err := id.ParseTemplateID(req.ID)
	if err != nil {
		return nil, forge.BadRequest("invalid template ID")
	}
	t, err := a.store.GetTemplate(ctx.Context(), tid)
	if err != nil {
		return nil, mapError(err)
	}
	return t, nil
}

func (a *ForgeAPI) updateTemplate(ctx forge.Context, req *UpdateTemplateRequest) (*template.Template, error) {
	tid, err := id.ParseTemplateID(req.ID)
	if err != nil {
		return nil, forge.BadRequest("invalid template ID")
	}
	existing, err := a.store.GetTemplate(ctx.Context(), tid)
	if err != nil {
		return nil, mapError(err)
	}
	if req.Slug != "" {
		existing.Slug = req.Slug
	}
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Channel != "" {
		existing.Channel = req.Channel
	}
	if req.Category != "" {
		existing.Category = req.Category
	}
	if req.Variables != nil {
		existing.Variables = req.Variables
	}
	existing.Enabled = req.Enabled
	existing.UpdatedAt = time.Now().UTC()
	if err := a.store.UpdateTemplate(ctx.Context(), existing); err != nil {
		return nil, mapError(err)
	}
	a.herald.Audit(ctx.Context(), bridge.SeverityInfo, bridge.OutcomeSuccess, "template.update", "template", existing.ID.String(), "", existing.AppID, "template", map[string]string{
		"slug": existing.Slug, "channel": existing.Channel,
	})
	return existing, nil
}

func (a *ForgeAPI) deleteTemplate(ctx forge.Context, req *DeleteTemplateRequest) (*template.Template, error) {
	tid, err := id.ParseTemplateID(req.ID)
	if err != nil {
		return nil, forge.BadRequest("invalid template ID")
	}
	if err := a.store.DeleteTemplate(ctx.Context(), tid); err != nil {
		return nil, mapError(err)
	}
	a.herald.Audit(ctx.Context(), bridge.SeverityWarning, bridge.OutcomeSuccess, "template.delete", "template", tid.String(), "", "", "template", nil)
	if err := ctx.NoContent(http.StatusNoContent); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

func (a *ForgeAPI) createVersion(ctx forge.Context, req *CreateVersionRequest) (*template.Version, error) {
	templateID, err := id.ParseTemplateID(req.TemplateID)
	if err != nil {
		return nil, forge.BadRequest("invalid template ID")
	}
	now := time.Now().UTC()
	v := &template.Version{
		ID:         id.NewTemplateVersionID(),
		TemplateID: templateID,
		Locale:     req.Locale,
		Subject:    req.Subject,
		HTML:       req.HTML,
		Text:       req.Text,
		Title:      req.Title,
		Active:     true,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := a.store.CreateVersion(ctx.Context(), v); err != nil {
		return nil, mapError(err)
	}
	a.herald.Audit(ctx.Context(), bridge.SeverityInfo, bridge.OutcomeSuccess, "template_version.create", "template_version", v.ID.String(), "", "", "template", map[string]string{
		"template_id": req.TemplateID, "locale": req.Locale,
	})
	if err := ctx.JSON(http.StatusCreated, v); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

func (a *ForgeAPI) listVersions(ctx forge.Context, req *ListVersionsRequest) (*struct{}, error) {
	tid, err := id.ParseTemplateID(req.TemplateID)
	if err != nil {
		return nil, forge.BadRequest("invalid template ID")
	}
	versions, err := a.store.ListVersions(ctx.Context(), tid)
	if err != nil {
		return nil, mapError(err)
	}
	if err := ctx.JSON(http.StatusOK, versions); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

func (a *ForgeAPI) updateVersion(ctx forge.Context, req *UpdateVersionRequest) (*template.Version, error) {
	vid, err := id.ParseTemplateVersionID(req.VersionID)
	if err != nil {
		return nil, forge.BadRequest("invalid version ID")
	}
	existing, err := a.store.GetVersion(ctx.Context(), vid)
	if err != nil {
		return nil, mapError(err)
	}
	if req.Locale != "" {
		existing.Locale = req.Locale
	}
	if req.Subject != "" {
		existing.Subject = req.Subject
	}
	if req.HTML != "" {
		existing.HTML = req.HTML
	}
	if req.Text != "" {
		existing.Text = req.Text
	}
	if req.Title != "" {
		existing.Title = req.Title
	}
	if req.Active != nil {
		existing.Active = *req.Active
	}
	existing.UpdatedAt = time.Now().UTC()
	if err := a.store.UpdateVersion(ctx.Context(), existing); err != nil {
		return nil, mapError(err)
	}
	a.herald.Audit(ctx.Context(), bridge.SeverityInfo, bridge.OutcomeSuccess, "template_version.update", "template_version", existing.ID.String(), "", "", "template", map[string]string{
		"locale": existing.Locale,
	})
	return existing, nil
}

func (a *ForgeAPI) deleteVersion(ctx forge.Context, req *DeleteVersionRequest) (*template.Version, error) {
	vid, err := id.ParseTemplateVersionID(req.VersionID)
	if err != nil {
		return nil, forge.BadRequest("invalid version ID")
	}
	if err := a.store.DeleteVersion(ctx.Context(), vid); err != nil {
		return nil, mapError(err)
	}
	a.herald.Audit(ctx.Context(), bridge.SeverityWarning, bridge.OutcomeSuccess, "template_version.delete", "template_version", vid.String(), "", "", "template", nil)
	if err := ctx.NoContent(http.StatusNoContent); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

// ─── Send Handlers ──────────────────────────────

func (a *ForgeAPI) sendNotification(ctx forge.Context, req *herald.SendRequest) (*herald.SendResult, error) {
	result, err := a.herald.Send(ctx.Context(), req)
	if err != nil {
		return nil, mapError(err)
	}
	return result, nil
}

func (a *ForgeAPI) notifyMultiChannel(ctx forge.Context, req *herald.NotifyRequest) (*struct{}, error) {
	results, err := a.herald.Notify(ctx.Context(), req)
	if err != nil {
		return nil, mapError(err)
	}
	if err := ctx.JSON(http.StatusOK, results); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

// ─── Message Handlers ──────────────────────────────

func (a *ForgeAPI) listMessages(ctx forge.Context, req *ListMessagesRequest) (*struct{}, error) {
	limit := req.Limit
	if limit == 0 {
		limit = 50
	}
	messages, err := a.store.ListMessages(ctx.Context(), req.AppID, message.ListOptions{
		Channel: req.Channel,
		Status:  message.Status(req.Status),
		Offset:  req.Offset,
		Limit:   limit,
	})
	if err != nil {
		return nil, mapError(err)
	}
	if err := ctx.JSON(http.StatusOK, messages); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

func (a *ForgeAPI) getMessage(ctx forge.Context, req *GetMessageRequest) (*message.Message, error) {
	mid, err := id.ParseMessageID(req.ID)
	if err != nil {
		return nil, forge.BadRequest("invalid message ID")
	}
	msg, err := a.store.GetMessage(ctx.Context(), mid)
	if err != nil {
		return nil, mapError(err)
	}
	return msg, nil
}

// ─── Inbox Handlers ──────────────────────────────

func (a *ForgeAPI) listInbox(ctx forge.Context, req *ListInboxRequest) (*struct{}, error) {
	limit := req.Limit
	if limit == 0 {
		limit = 50
	}
	notifs, err := a.store.ListNotifications(ctx.Context(), req.AppID, req.UserID, limit, req.Offset)
	if err != nil {
		return nil, mapError(err)
	}
	// Write JSON directly for the inbox notification type
	if err := ctx.JSON(http.StatusOK, notifs); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

func (a *ForgeAPI) unreadCount(ctx forge.Context, req *UnreadCountRequest) (*UnreadCountResponse, error) {
	count, err := a.store.UnreadCount(ctx.Context(), req.AppID, req.UserID)
	if err != nil {
		return nil, mapError(err)
	}
	return &UnreadCountResponse{Count: count}, nil
}

func (a *ForgeAPI) markRead(ctx forge.Context, req *MarkReadRequest) (*struct{}, error) {
	nid, err := id.ParseInboxID(req.ID)
	if err != nil {
		return nil, forge.BadRequest("invalid notification ID")
	}
	if err := a.store.MarkRead(ctx.Context(), nid); err != nil {
		return nil, mapError(err)
	}
	if err := ctx.NoContent(http.StatusNoContent); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

func (a *ForgeAPI) markAllRead(ctx forge.Context, req *MarkAllReadRequest) (*struct{}, error) {
	if err := a.store.MarkAllRead(ctx.Context(), req.AppID, req.UserID); err != nil {
		return nil, mapError(err)
	}
	if err := ctx.NoContent(http.StatusNoContent); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

func (a *ForgeAPI) deleteInboxItem(ctx forge.Context, req *DeleteInboxRequest) (*struct{}, error) {
	nid, err := id.ParseInboxID(req.ID)
	if err != nil {
		return nil, forge.BadRequest("invalid notification ID")
	}
	if err := a.store.DeleteNotification(ctx.Context(), nid); err != nil {
		return nil, mapError(err)
	}
	if err := ctx.NoContent(http.StatusNoContent); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

// ─── Preference Handlers ──────────────────────────────

func (a *ForgeAPI) getPreferences(ctx forge.Context, req *GetPreferencesRequest) (*preference.Preference, error) {
	pref, err := a.store.GetPreference(ctx.Context(), req.AppID, req.UserID)
	if err != nil {
		return nil, mapError(err)
	}
	return pref, nil
}

func (a *ForgeAPI) updatePreferences(ctx forge.Context, req *UpdatePreferencesRequest) (*preference.Preference, error) {
	pref := &preference.Preference{
		ID:        id.NewPreferenceID(),
		AppID:     req.AppID,
		UserID:    req.UserID,
		Overrides: req.Overrides,
		UpdatedAt: time.Now().UTC(),
	}
	if err := a.store.SetPreference(ctx.Context(), pref); err != nil {
		return nil, mapError(err)
	}
	a.herald.Audit(ctx.Context(), bridge.SeverityInfo, bridge.OutcomeSuccess, "preference.update", "preference", pref.ID.String(), req.UserID, req.AppID, "preference", nil)
	return pref, nil
}

// ─── Config Handlers ──────────────────────────────

func (a *ForgeAPI) getConfig(ctx forge.Context, req *GetConfigRequest) (*struct{}, error) {
	configs, err := a.store.ListScopedConfigs(ctx.Context(), req.AppID)
	if err != nil {
		return nil, mapError(err)
	}
	if err := ctx.JSON(http.StatusOK, configs); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

func (a *ForgeAPI) setAppConfig(ctx forge.Context, req *SetAppConfigRequest) (*scope.Config, error) {
	return a.saveScopedConfig(ctx, req.AppID, scope.ScopeApp, req.AppID,
		req.EmailProviderID, req.SMSProviderID, req.PushProviderID,
		req.FromEmail, req.FromName, req.FromPhone, req.DefaultLocale)
}

func (a *ForgeAPI) setOrgConfig(ctx forge.Context, req *SetOrgConfigRequest) (*scope.Config, error) {
	return a.saveScopedConfig(ctx, req.AppID, scope.ScopeOrg, req.OrgID,
		req.EmailProviderID, req.SMSProviderID, req.PushProviderID,
		req.FromEmail, req.FromName, req.FromPhone, req.DefaultLocale)
}

func (a *ForgeAPI) setUserConfig(ctx forge.Context, req *SetUserConfigRequest) (*scope.Config, error) {
	return a.saveScopedConfig(ctx, req.AppID, scope.ScopeUser, req.UserID,
		req.EmailProviderID, req.SMSProviderID, req.PushProviderID,
		req.FromEmail, req.FromName, req.FromPhone, req.DefaultLocale)
}

func (a *ForgeAPI) saveScopedConfig(
	ctx forge.Context,
	appID string, scopeType scope.ScopeType, scopeID string,
	emailPID, smsPID, pushPID, fromEmail, fromName, fromPhone, defaultLocale string,
) (*scope.Config, error) {
	now := time.Now().UTC()
	cfg := &scope.Config{
		ID:              id.NewScopedConfigID(),
		AppID:           appID,
		Scope:           scopeType,
		ScopeID:         scopeID,
		EmailProviderID: emailPID,
		SMSProviderID:   smsPID,
		PushProviderID:  pushPID,
		FromEmail:       fromEmail,
		FromName:        fromName,
		FromPhone:       fromPhone,
		DefaultLocale:   defaultLocale,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := a.store.SetScopedConfig(ctx.Context(), cfg); err != nil {
		return nil, mapError(err)
	}
	a.herald.Audit(ctx.Context(), bridge.SeverityInfo, bridge.OutcomeSuccess, "config.set", "scoped_config", cfg.ID.String(), "", appID, "config", map[string]string{
		"scope": string(scopeType), "scope_id": scopeID,
	})
	return cfg, nil
}

func (a *ForgeAPI) deleteOrgConfig(ctx forge.Context, req *DeleteOrgConfigRequest) (*scope.Config, error) {
	cfg, err := a.store.GetScopedConfig(ctx.Context(), req.AppID, scope.ScopeOrg, req.OrgID)
	if err != nil {
		return nil, mapError(err)
	}
	if err := a.store.DeleteScopedConfig(ctx.Context(), cfg.ID); err != nil {
		return nil, mapError(err)
	}
	a.herald.Audit(ctx.Context(), bridge.SeverityWarning, bridge.OutcomeSuccess, "config.delete", "scoped_config", cfg.ID.String(), "", req.AppID, "config", map[string]string{
		"scope": "org", "scope_id": req.OrgID,
	})
	if err := ctx.NoContent(http.StatusNoContent); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

func (a *ForgeAPI) deleteUserConfig(ctx forge.Context, req *DeleteUserConfigRequest) (*scope.Config, error) {
	cfg, err := a.store.GetScopedConfig(ctx.Context(), req.AppID, scope.ScopeUser, req.UserID)
	if err != nil {
		return nil, mapError(err)
	}
	if err := a.store.DeleteScopedConfig(ctx.Context(), cfg.ID); err != nil {
		return nil, mapError(err)
	}
	a.herald.Audit(ctx.Context(), bridge.SeverityWarning, bridge.OutcomeSuccess, "config.delete", "scoped_config", cfg.ID.String(), "", req.AppID, "config", map[string]string{
		"scope": "user", "scope_id": req.UserID,
	})
	if err := ctx.NoContent(http.StatusNoContent); err != nil {
		return nil, err
	}
	return nil, nil //nolint:nilnil // response already sent via ctx
}

// ─── Helpers ──────────────────────────────

func mapError(err error) error {
	if err == nil {
		return nil
	}
	return forge.InternalError(err)
}
