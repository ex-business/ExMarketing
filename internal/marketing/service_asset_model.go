package marketing

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type ServiceType string
type ServiceAssetScope string
type ServiceAssetStatus string
type ServiceInterfaceOperation string
type ServiceProviderProtocol string

const (
	ServiceTypeInternal ServiceType = "INTERNAL"
	ServiceTypeExternal ServiceType = "EXTERNAL"
	ServiceTypeSystem   ServiceType = "SYSTEM"

	ServiceAssetScopeSystem    ServiceAssetScope = "SYSTEM"
	ServiceAssetScopeTenant    ServiceAssetScope = "TENANT"
	ServiceAssetScopeWorkspace ServiceAssetScope = "WORKSPACE"
	ServiceAssetScopeFolder    ServiceAssetScope = "FOLDER"

	ServiceAssetActive   ServiceAssetStatus = "ACTIVE"
	ServiceAssetDraft    ServiceAssetStatus = "DRAFT"
	ServiceAssetDisabled ServiceAssetStatus = "DISABLED"
	ServiceAssetDeleted  ServiceAssetStatus = "DELETED"

	ServiceOperationCall             ServiceInterfaceOperation = "CALL"
	ServiceOperationPublish          ServiceInterfaceOperation = "PUBLISH"
	ServiceOperationRequestReply     ServiceInterfaceOperation = "REQUEST_REPLY"
	ServiceOperationSubscribeTrigger ServiceInterfaceOperation = "SUBSCRIBE_TRIGGER"

	ServiceProviderHTTP       ServiceProviderProtocol = "HTTP"
	ServiceProviderRPC        ServiceProviderProtocol = "RPC"
	ServiceProviderMQ         ServiceProviderProtocol = "MQ"
	ServiceProviderPlugin     ServiceProviderProtocol = "PLUGIN"
	ServiceProviderTCP        ServiceProviderProtocol = "TCP"
	ServiceProviderLuaAdapter ServiceProviderProtocol = "LUA_ADAPTER"
	ServiceProviderResource   ServiceProviderProtocol = "RESOURCE"
)

type ServiceDefinition struct {
	ID          string             `json:"id"`
	TenantID    string             `json:"tenantId"`
	WorkspaceID string             `json:"workspaceId,omitempty"`
	FolderID    string             `json:"folderId,omitempty"`
	Code        string             `json:"code"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	ServiceType ServiceType        `json:"serviceType"`
	Scope       ServiceAssetScope  `json:"scope"`
	Status      ServiceAssetStatus `json:"status"`
	Owner       string             `json:"owner,omitempty"`
	Tags        []string           `json:"tags,omitempty"`
	Metadata    json.RawMessage    `json:"metadata,omitempty"`
	CreatedBy   string             `json:"createdBy,omitempty"`
	UpdatedBy   string             `json:"updatedBy,omitempty"`
	CreatedAt   time.Time          `json:"createdAt"`
	UpdatedAt   time.Time          `json:"updatedAt"`
}

type ServiceInterface struct {
	ID                     string                    `json:"id"`
	TenantID               string                    `json:"tenantId"`
	ServiceRef             string                    `json:"serviceRef"`
	Code                   string                    `json:"code"`
	Name                   string                    `json:"name"`
	Description            string                    `json:"description,omitempty"`
	Operation              ServiceInterfaceOperation `json:"operation"`
	InputSchemaRef         string                    `json:"inputSchemaRef,omitempty"`
	OutputSchemaRef        string                    `json:"outputSchemaRef,omitempty"`
	ErrorSchemaRef         string                    `json:"errorSchemaRef,omitempty"`
	IdempotencyKeyTemplate string                    `json:"idempotencyKeyTemplate,omitempty"`
	TimeoutMS              int                       `json:"timeoutMs"`
	RetryPolicy            string                    `json:"retryPolicy,omitempty"`
	LuaCallable            bool                      `json:"luaCallable"`
	Status                 ServiceAssetStatus        `json:"status"`
	Metadata               json.RawMessage           `json:"metadata,omitempty"`
	CreatedBy              string                    `json:"createdBy,omitempty"`
	UpdatedBy              string                    `json:"updatedBy,omitempty"`
	CreatedAt              time.Time                 `json:"createdAt"`
	UpdatedAt              time.Time                 `json:"updatedAt"`
}

type ServiceProvider struct {
	ID           string                  `json:"id"`
	TenantID     string                  `json:"tenantId"`
	ServiceRef   string                  `json:"serviceRef"`
	InterfaceRef string                  `json:"interfaceRef"`
	Code         string                  `json:"code"`
	Name         string                  `json:"name"`
	Description  string                  `json:"description,omitempty"`
	Protocol     ServiceProviderProtocol `json:"protocol"`
	Endpoint     string                  `json:"endpoint,omitempty"`
	Topic        string                  `json:"topic,omitempty"`
	Method       string                  `json:"method,omitempty"`
	Path         string                  `json:"path,omitempty"`
	AuthRef      string                  `json:"authRef,omitempty"`
	ResourceRefs []string                `json:"resourceRefs,omitempty"`
	Config       json.RawMessage         `json:"config,omitempty"`
	Weight       int                     `json:"weight,omitempty"`
	Priority     int                     `json:"priority,omitempty"`
	Environment  string                  `json:"environment,omitempty"`
	Status       ServiceAssetStatus      `json:"status"`
	CreatedBy    string                  `json:"createdBy,omitempty"`
	UpdatedBy    string                  `json:"updatedBy,omitempty"`
	CreatedAt    time.Time               `json:"createdAt"`
	UpdatedAt    time.Time               `json:"updatedAt"`
}

type ServiceDefinitionQuery struct {
	TenantID    string
	WorkspaceID string
	FolderID    string
	Status      ServiceAssetStatus
	Code        string
	Limit       int
}

type ServiceInterfaceQuery struct {
	TenantID   string
	ServiceRef string
	Status     ServiceAssetStatus
	Code       string
	Limit      int
}

type ServiceProviderQuery struct {
	TenantID     string
	ServiceRef   string
	InterfaceRef string
	Protocol     ServiceProviderProtocol
	Status       ServiceAssetStatus
	Code         string
	Limit        int
}

func (s ServiceDefinition) Validate() error {
	if strings.TrimSpace(s.TenantID) == "" {
		return errors.Join(ErrInvalidResource, errors.New("tenantId is required"))
	}
	if strings.TrimSpace(s.Code) == "" {
		return errors.Join(ErrInvalidResource, errors.New("code is required"))
	}
	if strings.TrimSpace(s.Name) == "" {
		return errors.Join(ErrInvalidResource, errors.New("name is required"))
	}
	if !validServiceType(s.ServiceType) {
		return errors.Join(ErrInvalidResource, errors.New("serviceType is invalid"))
	}
	if !validServiceAssetScope(s.Scope) {
		return errors.Join(ErrInvalidResource, errors.New("scope is invalid"))
	}
	if !validServiceAssetStatus(s.Status) {
		return errors.Join(ErrInvalidResource, errors.New("status is invalid"))
	}
	if len(s.Metadata) > 0 && string(s.Metadata) != "{}" && !jsonObjectValid(s.Metadata) {
		return errors.Join(ErrInvalidResource, errors.New("metadata must be json object"))
	}
	return nil
}

func (i ServiceInterface) Validate() error {
	if strings.TrimSpace(i.TenantID) == "" {
		return errors.Join(ErrInvalidResource, errors.New("tenantId is required"))
	}
	if strings.TrimSpace(i.ServiceRef) == "" {
		return errors.Join(ErrInvalidResource, errors.New("serviceRef is required"))
	}
	if strings.TrimSpace(i.Code) == "" {
		return errors.Join(ErrInvalidResource, errors.New("code is required"))
	}
	if strings.TrimSpace(i.Name) == "" {
		return errors.Join(ErrInvalidResource, errors.New("name is required"))
	}
	if !validServiceInterfaceOperation(i.Operation) {
		return errors.Join(ErrInvalidResource, errors.New("operation is invalid"))
	}
	if i.TimeoutMS < 0 {
		return errors.Join(ErrInvalidResource, errors.New("timeoutMs must be non-negative"))
	}
	if !validServiceAssetStatus(i.Status) {
		return errors.Join(ErrInvalidResource, errors.New("status is invalid"))
	}
	if len(i.Metadata) > 0 && string(i.Metadata) != "{}" && !jsonObjectValid(i.Metadata) {
		return errors.Join(ErrInvalidResource, errors.New("metadata must be json object"))
	}
	return nil
}

func (p ServiceProvider) Validate() error {
	if strings.TrimSpace(p.TenantID) == "" {
		return errors.Join(ErrInvalidResource, errors.New("tenantId is required"))
	}
	if strings.TrimSpace(p.ServiceRef) == "" {
		return errors.Join(ErrInvalidResource, errors.New("serviceRef is required"))
	}
	if strings.TrimSpace(p.InterfaceRef) == "" {
		return errors.Join(ErrInvalidResource, errors.New("interfaceRef is required"))
	}
	if strings.TrimSpace(p.Code) == "" {
		return errors.Join(ErrInvalidResource, errors.New("code is required"))
	}
	if strings.TrimSpace(p.Name) == "" {
		return errors.Join(ErrInvalidResource, errors.New("name is required"))
	}
	if !validServiceProviderProtocol(p.Protocol) {
		return errors.Join(ErrInvalidResource, errors.New("protocol is invalid"))
	}
	if !validServiceAssetStatus(p.Status) {
		return errors.Join(ErrInvalidResource, errors.New("status is invalid"))
	}
	if len(p.Config) > 0 && string(p.Config) != "{}" && !jsonObjectValid(p.Config) {
		return errors.Join(ErrInvalidResource, errors.New("config must be json object"))
	}
	return nil
}

func validServiceType(value ServiceType) bool {
	switch value {
	case ServiceTypeInternal, ServiceTypeExternal, ServiceTypeSystem:
		return true
	default:
		return false
	}
}

func validServiceAssetScope(value ServiceAssetScope) bool {
	switch value {
	case ServiceAssetScopeSystem, ServiceAssetScopeTenant, ServiceAssetScopeWorkspace, ServiceAssetScopeFolder:
		return true
	default:
		return false
	}
}

func validServiceAssetStatus(value ServiceAssetStatus) bool {
	switch value {
	case ServiceAssetActive, ServiceAssetDraft, ServiceAssetDisabled, ServiceAssetDeleted:
		return true
	default:
		return false
	}
}

func validServiceInterfaceOperation(value ServiceInterfaceOperation) bool {
	switch value {
	case ServiceOperationCall, ServiceOperationPublish, ServiceOperationRequestReply, ServiceOperationSubscribeTrigger:
		return true
	default:
		return false
	}
}

func validServiceProviderProtocol(value ServiceProviderProtocol) bool {
	switch value {
	case ServiceProviderHTTP, ServiceProviderRPC, ServiceProviderMQ, ServiceProviderPlugin, ServiceProviderTCP, ServiceProviderLuaAdapter, ServiceProviderResource:
		return true
	default:
		return false
	}
}

func jsonObjectValid(raw json.RawMessage) bool {
	var mapped map[string]any
	return json.Unmarshal(raw, &mapped) == nil && mapped != nil
}
