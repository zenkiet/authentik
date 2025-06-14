package search

import (
	"context"
	"fmt"
	"net"
	"strings"

	"beryju.io/ldap"
	"github.com/getsentry/sentry-go"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"goauthentik.io/internal/utils"
)

type Request struct {
	ldap.SearchRequest
	BindDN            string
	FilterObjectClass string
	log               *log.Entry

	id   string
	conn net.Conn
	ctx  context.Context
}

func NewRequest(bindDN string, searchReq ldap.SearchRequest, conn net.Conn) (*Request, *sentry.Span) {
	rid := uuid.New().String()
	bindDN = strings.ToLower(bindDN)
	span := sentry.StartSpan(context.TODO(), "authentik.providers.ldap.search", sentry.WithTransactionName("authentik.providers.ldap.search"))
	span.Description = fmt.Sprintf("%s (%s)", searchReq.BaseDN, ldap.ScopeMap[searchReq.Scope])
	span.SetTag("request_uid", rid)
	hub := sentry.GetHubFromContext(span.Context())
	if hub == nil {
		hub = sentry.CurrentHub()
	}
	hub.Scope().SetUser(sentry.User{
		Username:  bindDN,
		ID:        bindDN,
		IPAddress: utils.GetIP(conn.RemoteAddr()),
	})
	span.SetTag("ldap_filter", searchReq.Filter)
	span.SetTag("ldap_base_dn", searchReq.BaseDN)
	l := log.WithFields(log.Fields{
		"bindDN":    bindDN,
		"baseDN":    searchReq.BaseDN,
		"requestId": rid,
		"scope":     ldap.ScopeMap[searchReq.Scope],
		"client":    utils.GetIP(conn.RemoteAddr()),
		"filter":    searchReq.Filter,
	})
	filterOC, err := ldap.GetFilterObjectClass(searchReq.Filter)
	if err != nil && len(searchReq.Filter) > 0 {
		l.WithError(err).WithField("objectClass", filterOC).Warning("invalid filter object class")
	}

	// Handle comma-separated attributes
	normalizedAttributes := normalizeAttributes(searchReq.Attributes)
	if len(normalizedAttributes) != len(searchReq.Attributes) {
		// Create a copy of the search request with normalized attributes
		searchReq.Attributes = normalizedAttributes
	}

	return &Request{
		SearchRequest:     searchReq,
		BindDN:            bindDN,
		FilterObjectClass: filterOC,
		conn:              conn,
		log:               l,
		id:                rid,
		ctx:               span.Context(),
	}, span
}

// normalizeAttributes handles the case where attributes might be passed as comma-separated strings
// rather than as individual array elements
func normalizeAttributes(attributes []string) []string {
	if len(attributes) == 0 {
		return attributes
	}

	result := make([]string, 0, len(attributes))
	for _, attr := range attributes {
		if strings.Contains(attr, ",") {
			// Split comma-separated attributes and add them individually
			parts := strings.Split(attr, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					result = append(result, part)
				}
			}
		} else {
			result = append(result, attr)
		}
	}
	return result
}

func (r *Request) Context() context.Context {
	return r.ctx
}

func (r *Request) Log() *log.Entry {
	return r.log
}

func (r *Request) RemoteAddr() string {
	return utils.GetIP(r.conn.RemoteAddr())
}
