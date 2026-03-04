package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Alerts ---

func TestListAlertsCLI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/v1/alerts") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "alert-1",
					"attributes": map[string]interface{}{
						"short_id":   "ALT-1A2B",
						"summary":    "High CPU",
						"status":     "triggered",
						"source":     "datadog",
						"created_at": "2025-06-15T10:00:00Z",
						"updated_at": "2025-06-15T10:00:00Z",
					},
				},
			},
			"meta": map[string]interface{}{
				"current_page": 1,
				"total_pages":  1,
				"total_count":  1,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.ListAlertsCLI(context.Background(), 1, 25, "", nil)
	if err != nil {
		t.Fatalf("ListAlertsCLI returned error: %v", err)
	}

	if len(result.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(result.Alerts))
	}
	if result.Alerts[0].ShortID != "ALT-1A2B" {
		t.Errorf("ShortID = %q, want %q", result.Alerts[0].ShortID, "ALT-1A2B")
	}
	if result.Alerts[0].Source != "datadog" {
		t.Errorf("Source = %q, want %q", result.Alerts[0].Source, "datadog")
	}
}

func TestListAlertsCLIPageSizeCap(t *testing.T) {
	var receivedPageSize string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPageSize = r.URL.Query().Get("page[size]")
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{},
			"meta": map[string]interface{}{"current_page": 1, "total_pages": 1, "total_count": 0},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	_, _ = client.ListAlertsCLI(context.Background(), 1, 0, "", nil)
	if receivedPageSize != "25" {
		t.Errorf("default page size = %q, want 25", receivedPageSize)
	}

	_, _ = client.ListAlertsCLI(context.Background(), 1, 200, "", nil)
	if receivedPageSize != "100" {
		t.Errorf("capped page size = %q, want 100", receivedPageSize)
	}
}

func TestListAlertsCLIWithFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		if !strings.Contains(query, "filter[status]=triggered") {
			t.Errorf("expected status filter, got query: %s", query)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{},
			"meta": map[string]interface{}{"current_page": 1, "total_pages": 1, "total_count": 0},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListAlertsCLI(context.Background(), 1, 25, "-created_at", map[string]string{"status": "triggered"})
	if err != nil {
		t.Fatalf("ListAlertsCLI returned error: %v", err)
	}
}

func TestListAlertsCLIUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListAlertsCLI(context.Background(), 1, 25, "", nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "invalid API token") {
		t.Errorf("error = %q, want 'invalid API token'", err.Error())
	}
}

func TestListAlertsCLIForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListAlertsCLI(context.Background(), 1, 25, "", nil)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("error = %q, want 'access denied'", err.Error())
	}
}

func TestListAlertsCLINotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListAlertsCLI(context.Background(), 1, 25, "", nil)
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestListAlertsCLIServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListAlertsCLI(context.Background(), 1, 25, "", nil)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("error = %q, want 'status 500'", err.Error())
	}
}

func TestGetAlertByID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/alerts/alert-42") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "alert-42",
				"attributes": map[string]interface{}{
					"short_id":    "ALT-42AB",
					"summary":     "DB Replica Lag",
					"description": "Lag over 30s",
					"status":      "acknowledged",
					"source":      "grafana",
					"created_at":  "2025-06-15T10:00:00Z",
					"updated_at":  "2025-06-15T11:00:00Z",
					"url":         "https://rootly.com/alerts/42",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	alert, err := client.GetAlertByID(context.Background(), "alert-42")
	if err != nil {
		t.Fatalf("GetAlertByID returned error: %v", err)
	}

	if alert.ShortID != "ALT-42AB" {
		t.Errorf("ShortID = %q, want %q", alert.ShortID, "ALT-42AB")
	}
	if alert.Summary != "DB Replica Lag" {
		t.Errorf("Summary = %q, want %q", alert.Summary, "DB Replica Lag")
	}
	if alert.URL != "https://rootly.com/alerts/42" {
		t.Errorf("URL = %q, want %q", alert.URL, "https://rootly.com/alerts/42")
	}
}

func TestGetAlertByIDNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetAlertByID(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "alert not found") {
		t.Errorf("error = %q, want 'alert not found'", err.Error())
	}
}

func TestGetAlertByIDFullDetail(t *testing.T) {
	seqID := 42
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "alert-full",
				"attributes": map[string]interface{}{
					"short_id":              "ALT-FULL",
					"summary":               "Full Detail Alert",
					"description":           "Alert with all fields",
					"status":                "triggered",
					"source":                "pagerduty",
					"external_url":          "https://pagerduty.com/alert/123",
					"created_at":            "2025-06-15T10:00:00Z",
					"updated_at":            "2025-06-15T11:00:00Z",
					"started_at":            "2025-06-15T10:01:00Z",
					"ended_at":              "2025-06-15T11:00:00Z",
					"url":                   "https://rootly.com/alerts/full",
					"external_id":           "ext-123",
					"noise":                 "not_noise",
					"is_group_leader_alert": true,
					"group_leader_alert_id": "leader-1",
					"deduplication_key":     "dedup-abc",
					"labels": []map[string]interface{}{
						{"key": "severity", "value": "critical"},
					},
					"services": []map[string]interface{}{
						{"name": "api-gateway"},
						{"name": "auth-service"},
					},
					"environments": []map[string]interface{}{
						{"name": "production"},
					},
					"groups": []map[string]interface{}{
						{"name": "Platform"},
					},
					"responders": []map[string]interface{}{
						{
							"id": "resp-1",
							"attributes": map[string]interface{}{
								"user": map[string]interface{}{
									"data": map[string]interface{}{
										"attributes": map[string]interface{}{
											"name": "Alice OnCall",
										},
									},
								},
							},
						},
					},
					"alert_urgency": map[string]interface{}{
						"data": map[string]interface{}{
							"attributes": map[string]interface{}{
								"name": "high",
							},
						},
					},
					"notified_users": []map[string]interface{}{
						{"name": "Bob Notified", "email": "bob@example.com"},
					},
					"incidents": []map[string]interface{}{
						{
							"id": "inc-rel",
							"attributes": map[string]interface{}{
								"sequential_id": seqID,
								"title":         "Related Incident",
								"status":        "started",
							},
						},
					},
					"data": map[string]interface{}{
						"custom_field": "custom_value",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	alert, err := client.GetAlertByID(context.Background(), "alert-full")
	if err != nil {
		t.Fatalf("GetAlertByID returned error: %v", err)
	}

	// Basic fields
	if alert.ShortID != "ALT-FULL" {
		t.Errorf("ShortID = %q, want %q", alert.ShortID, "ALT-FULL")
	}
	if alert.Source != "pagerduty" {
		t.Errorf("Source = %q, want %q", alert.Source, "pagerduty")
	}
	if alert.Description != "Alert with all fields" {
		t.Errorf("Description = %q", alert.Description)
	}
	if alert.ExternalURL != "https://pagerduty.com/alert/123" {
		t.Errorf("ExternalURL = %q", alert.ExternalURL)
	}
	if alert.URL != "https://rootly.com/alerts/full" {
		t.Errorf("URL = %q", alert.URL)
	}
	if alert.ExternalID != "ext-123" {
		t.Errorf("ExternalID = %q", alert.ExternalID)
	}
	if alert.Noise != "not_noise" {
		t.Errorf("Noise = %q", alert.Noise)
	}
	if !alert.IsGroupLeaderAlert {
		t.Error("IsGroupLeaderAlert = false, want true")
	}
	if alert.GroupLeaderAlertID != "leader-1" {
		t.Errorf("GroupLeaderAlertID = %q", alert.GroupLeaderAlertID)
	}
	if alert.DeduplicationKey != "dedup-abc" {
		t.Errorf("DeduplicationKey = %q", alert.DeduplicationKey)
	}
	if !alert.DetailLoaded {
		t.Error("DetailLoaded = false, want true")
	}

	// Labels
	if alert.Labels["severity"] != "critical" {
		t.Errorf("Labels[severity] = %q", alert.Labels["severity"])
	}

	// Collections
	if len(alert.Services) != 2 || alert.Services[0] != "api-gateway" {
		t.Errorf("Services = %v", alert.Services)
	}
	if len(alert.Environments) != 1 || alert.Environments[0] != "production" {
		t.Errorf("Environments = %v", alert.Environments)
	}
	if len(alert.Groups) != 1 || alert.Groups[0] != "Platform" {
		t.Errorf("Groups = %v", alert.Groups)
	}

	// Responders
	if len(alert.Responders) != 1 || alert.Responders[0] != "Alice OnCall" {
		t.Errorf("Responders = %v", alert.Responders)
	}

	// Urgency
	if alert.Urgency != "high" {
		t.Errorf("Urgency = %q, want %q", alert.Urgency, "high")
	}

	// Notified users
	if len(alert.NotifiedUsers) != 1 || alert.NotifiedUsers[0].Name != "Bob Notified" {
		t.Errorf("NotifiedUsers = %v", alert.NotifiedUsers)
	}

	// Related incidents
	if len(alert.RelatedIncidents) != 1 {
		t.Fatalf("RelatedIncidents count = %d, want 1", len(alert.RelatedIncidents))
	}
	if alert.RelatedIncidents[0].SequentialID != "INC-42" {
		t.Errorf("RelatedIncident SequentialID = %q, want %q", alert.RelatedIncidents[0].SequentialID, "INC-42")
	}

	// Timestamps
	if alert.StartedAt == nil {
		t.Error("StartedAt is nil, want non-nil")
	}
	if alert.EndedAt == nil {
		t.Error("EndedAt is nil, want non-nil")
	}

	// Custom data
	if alert.Data == nil || alert.Data["custom_field"] != "custom_value" {
		t.Errorf("Data = %v", alert.Data)
	}
}

// --- Services ---

func TestListServicesCLI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/services") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		desc := "Main API"
		color := "#FF5733"
		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "svc-1",
					"attributes": map[string]interface{}{
						"name":        "api-gateway",
						"slug":        "api-gateway",
						"description": desc,
						"color":       color,
						"created_at":  "2025-06-15T10:00:00Z",
						"updated_at":  "2025-06-15T10:00:00Z",
					},
				},
			},
			"meta": map[string]interface{}{
				"current_page": 1,
				"total_pages":  1,
				"total_count":  1,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.ListServicesCLI(context.Background(), 1, 25, "", nil)
	if err != nil {
		t.Fatalf("ListServicesCLI returned error: %v", err)
	}

	if len(result.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(result.Services))
	}
	if result.Services[0].Name != "api-gateway" {
		t.Errorf("Name = %q, want %q", result.Services[0].Name, "api-gateway")
	}
	if result.Services[0].Description != "Main API" {
		t.Errorf("Description = %q, want %q", result.Services[0].Description, "Main API")
	}
	if result.Services[0].Color != "#FF5733" {
		t.Errorf("Color = %q, want %q", result.Services[0].Color, "#FF5733")
	}
}

func TestListServicesCLIPageSizeCap(t *testing.T) {
	var receivedPageSize string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPageSize = r.URL.Query().Get("page[size]")
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{},
			"meta": map[string]interface{}{"current_page": 1, "total_pages": 1, "total_count": 0},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	_, _ = client.ListServicesCLI(context.Background(), 1, 0, "", nil)
	if receivedPageSize != "25" {
		t.Errorf("default page size = %q, want 25", receivedPageSize)
	}

	_, _ = client.ListServicesCLI(context.Background(), 1, 200, "", nil)
	if receivedPageSize != "100" {
		t.Errorf("capped page size = %q, want 100", receivedPageSize)
	}
}

func TestListServicesCLIWithFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.RawQuery
		if !strings.Contains(query, "filter[name]=api") {
			t.Errorf("expected name filter, got query: %s", query)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{},
			"meta": map[string]interface{}{"current_page": 1, "total_pages": 1, "total_count": 0},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListServicesCLI(context.Background(), 1, 25, "", map[string]string{"name": "api"})
	if err != nil {
		t.Fatalf("returned error: %v", err)
	}
}

func TestListServicesCLIUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListServicesCLI(context.Background(), 1, 25, "", nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestListServicesCLIForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListServicesCLI(context.Background(), 1, 25, "", nil)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("error = %q, want 'access denied'", err.Error())
	}
}

func TestListServicesCLIServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListServicesCLI(context.Background(), 1, 25, "", nil)
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestGetServiceByID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/services/svc-42") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "svc-42",
				"attributes": map[string]interface{}{
					"name":        "payments",
					"slug":        "payments",
					"description": "Payment service",
					"color":       "#00FF00",
					"created_at":  "2025-01-01T00:00:00Z",
					"updated_at":  "2025-06-15T10:00:00Z",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	svc, err := client.GetServiceByID(context.Background(), "svc-42")
	if err != nil {
		t.Fatalf("GetServiceByID returned error: %v", err)
	}

	if svc.Name != "payments" {
		t.Errorf("Name = %q, want %q", svc.Name, "payments")
	}
	if svc.Slug != "payments" {
		t.Errorf("Slug = %q, want %q", svc.Slug, "payments")
	}
	if svc.Description != "Payment service" {
		t.Errorf("Description = %q, want %q", svc.Description, "Payment service")
	}
}

func TestGetServiceByIDWithOwnerGroup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "svc-og",
				"attributes": map[string]interface{}{
					"name":        "auth-service",
					"slug":        "auth-service",
					"description": "Authentication",
					"color":       "#0000FF",
					"created_at":  "2025-01-01T00:00:00Z",
					"updated_at":  "2025-06-15T10:00:00Z",
				},
				"relationships": map[string]interface{}{
					"owner_group": map[string]interface{}{
						"data": map[string]interface{}{
							"id": "group-99",
						},
					},
				},
			},
			"included": []map[string]interface{}{
				{
					"id":   "group-99",
					"type": "groups",
					"attributes": map[string]interface{}{
						"name": "Platform Team",
					},
				},
				{
					"id":   "group-other",
					"type": "groups",
					"attributes": map[string]interface{}{
						"name": "Other Team",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	svc, err := client.GetServiceByID(context.Background(), "svc-og")
	if err != nil {
		t.Fatalf("GetServiceByID returned error: %v", err)
	}

	if svc.OwnerTeamName != "Platform Team" {
		t.Errorf("OwnerTeamName = %q, want %q", svc.OwnerTeamName, "Platform Team")
	}
	if svc.Color != "#0000FF" {
		t.Errorf("Color = %q, want %q", svc.Color, "#0000FF")
	}
	if !svc.DetailLoaded {
		t.Error("DetailLoaded = false, want true")
	}
}

func TestGetServiceByIDForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetServiceByID(context.Background(), "svc-1")
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("error = %q, want 'access denied'", err.Error())
	}
}

func TestDeleteService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteService(context.Background(), "svc-del")
	if err != nil {
		t.Fatalf("DeleteService returned error: %v", err)
	}
}

func TestDeleteServiceNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteService(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestDeleteServiceForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteService(context.Background(), "svc-1")
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("error = %q, want 'access denied'", err.Error())
	}
}

func TestDeleteServiceUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteService(context.Background(), "svc-1")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "invalid API token") {
		t.Errorf("error = %q, want 'invalid API token'", err.Error())
	}
}

func TestDeleteServiceServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteService(context.Background(), "svc-1")
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// --- Teams ---

func TestListTeamsCLI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/teams") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "team-1",
					"attributes": map[string]interface{}{
						"name":        "Platform",
						"slug":        "platform",
						"description": "Platform team",
						"color":       "#3498DB",
						"created_at":  "2025-06-15T10:00:00Z",
						"updated_at":  "2025-06-15T10:00:00Z",
					},
				},
				{
					"id": "team-2",
					"attributes": map[string]interface{}{
						"name":       "SRE",
						"slug":       "sre",
						"created_at": "2025-06-14T10:00:00Z",
						"updated_at": "2025-06-14T10:00:00Z",
					},
				},
			},
			"meta": map[string]interface{}{
				"current_page": 1,
				"total_pages":  1,
				"total_count":  2,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.ListTeamsCLI(context.Background(), 1, 25, "", nil)
	if err != nil {
		t.Fatalf("ListTeamsCLI returned error: %v", err)
	}

	if len(result.Teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(result.Teams))
	}
	if result.Teams[0].Name != "Platform" {
		t.Errorf("Name = %q, want %q", result.Teams[0].Name, "Platform")
	}
	if result.Teams[0].Color != "#3498DB" {
		t.Errorf("Color = %q, want %q", result.Teams[0].Color, "#3498DB")
	}
	// Second team has no description/color
	if result.Teams[1].Description != "" {
		t.Errorf("Description = %q, want empty", result.Teams[1].Description)
	}
}

func TestListTeamsCLIPageSizeCap(t *testing.T) {
	var receivedPageSize string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPageSize = r.URL.Query().Get("page[size]")
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{},
			"meta": map[string]interface{}{"current_page": 1, "total_pages": 1, "total_count": 0},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	_, _ = client.ListTeamsCLI(context.Background(), 1, 0, "", nil)
	if receivedPageSize != "25" {
		t.Errorf("default page size = %q, want 25", receivedPageSize)
	}

	_, _ = client.ListTeamsCLI(context.Background(), 1, 200, "", nil)
	if receivedPageSize != "100" {
		t.Errorf("capped page size = %q, want 100", receivedPageSize)
	}
}

func TestListTeamsCLIUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListTeamsCLI(context.Background(), 1, 25, "", nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestListTeamsCLIForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListTeamsCLI(context.Background(), 1, 25, "", nil)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("error = %q, want 'access denied'", err.Error())
	}
}

func TestListTeamsCLIServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListTeamsCLI(context.Background(), 1, 25, "", nil)
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestGetTeamByID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/teams/team-42") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "team-42",
				"attributes": map[string]interface{}{
					"name":        "Backend",
					"slug":        "backend",
					"description": "Backend engineers",
					"created_at":  "2025-01-01T00:00:00Z",
					"updated_at":  "2025-06-15T10:00:00Z",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	team, err := client.GetTeamByID(context.Background(), "team-42")
	if err != nil {
		t.Fatalf("GetTeamByID returned error: %v", err)
	}

	if team.Name != "Backend" {
		t.Errorf("Name = %q, want %q", team.Name, "Backend")
	}
	if team.Slug != "backend" {
		t.Errorf("Slug = %q, want %q", team.Slug, "backend")
	}
}

func TestGetTeamByIDWithUsers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "team-users",
				"attributes": map[string]interface{}{
					"name":        "Platform",
					"slug":        "platform",
					"description": "Platform team",
					"color":       "#FF0000",
					"created_at":  "2025-01-01T00:00:00Z",
					"updated_at":  "2025-06-15T10:00:00Z",
				},
			},
			"included": []map[string]interface{}{
				{
					"type": "users",
					"id":   "user-1",
					"attributes": map[string]interface{}{
						"full_name": "Alice Smith",
						"email":     "alice@example.com",
					},
				},
				{
					"type": "users",
					"id":   "user-2",
					"attributes": map[string]interface{}{
						"full_name": "",
						"email":     "bob@example.com",
					},
				},
				{
					"type": "services",
					"id":   "svc-1",
					"attributes": map[string]interface{}{
						"full_name": "Not a user",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	team, err := client.GetTeamByID(context.Background(), "team-users")
	if err != nil {
		t.Fatalf("GetTeamByID returned error: %v", err)
	}

	if !team.DetailLoaded {
		t.Error("DetailLoaded = false, want true")
	}
	if team.Color != "#FF0000" {
		t.Errorf("Color = %q, want %q", team.Color, "#FF0000")
	}
	if team.Description != "Platform team" {
		t.Errorf("Description = %q, want %q", team.Description, "Platform team")
	}
	// Should have 2 users (not the service entry)
	if len(team.Users) != 2 {
		t.Fatalf("Users count = %d, want 2", len(team.Users))
	}
	if team.Users[0] != "Alice Smith" {
		t.Errorf("Users[0] = %q, want %q", team.Users[0], "Alice Smith")
	}
	// User with empty full_name should fall back to email
	if team.Users[1] != "bob@example.com" {
		t.Errorf("Users[1] = %q, want %q (email fallback)", team.Users[1], "bob@example.com")
	}
}

func TestGetTeamByIDForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetTeamByID(context.Background(), "team-1")
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("error = %q, want 'access denied'", err.Error())
	}
}

func TestDeleteTeam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteTeam(context.Background(), "team-del")
	if err != nil {
		t.Fatalf("DeleteTeam returned error: %v", err)
	}
}

func TestDeleteTeamNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteTeam(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "team not found") {
		t.Errorf("error = %q, want 'team not found'", err.Error())
	}
}

func TestDeleteTeamForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteTeam(context.Background(), "team-1")
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("error = %q, want 'access denied'", err.Error())
	}
}

func TestDeleteTeamUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteTeam(context.Background(), "team-1")
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "invalid API token") {
		t.Errorf("error = %q, want 'invalid API token'", err.Error())
	}
}

func TestDeleteTeamServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.DeleteTeam(context.Background(), "team-1")
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// --- Schedules ---

func TestListSchedulesCLI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/on_call_schedules") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "sched-1",
					"attributes": map[string]interface{}{
						"name":        "Primary On-Call",
						"description": "Primary rotation",
						"created_at":  "2025-01-01T00:00:00Z",
						"updated_at":  "2025-06-15T10:00:00Z",
					},
				},
			},
			"meta": map[string]interface{}{
				"current_page": 1,
				"total_pages":  1,
				"total_count":  1,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.ListSchedulesCLI(context.Background(), 1, 25, nil)
	if err != nil {
		t.Fatalf("ListSchedulesCLI returned error: %v", err)
	}

	if len(result.Schedules) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(result.Schedules))
	}
	if result.Schedules[0].Name != "Primary On-Call" {
		t.Errorf("Name = %q, want %q", result.Schedules[0].Name, "Primary On-Call")
	}
	if result.Schedules[0].Description != "Primary rotation" {
		t.Errorf("Description = %q, want %q", result.Schedules[0].Description, "Primary rotation")
	}
}

func TestListSchedulesCLIPageSizeCap(t *testing.T) {
	var receivedPageSize string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPageSize = r.URL.Query().Get("page[size]")
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{},
			"meta": map[string]interface{}{"current_page": 1, "total_pages": 1, "total_count": 0},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	_, _ = client.ListSchedulesCLI(context.Background(), 1, 0, nil)
	if receivedPageSize != "25" {
		t.Errorf("default page size = %q, want 25", receivedPageSize)
	}

	_, _ = client.ListSchedulesCLI(context.Background(), 1, 200, nil)
	if receivedPageSize != "100" {
		t.Errorf("capped page size = %q, want 100", receivedPageSize)
	}
}

func TestListSchedulesCLIUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListSchedulesCLI(context.Background(), 1, 25, nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestListSchedulesCLIForbidden(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListSchedulesCLI(context.Background(), 1, 25, nil)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("error = %q, want 'access denied'", err.Error())
	}
}

func TestListSchedulesCLIServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListSchedulesCLI(context.Background(), 1, 25, nil)
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// --- Shifts ---

func TestListShiftsCLI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/v1/shifts") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "shift-1",
					"attributes": map[string]interface{}{
						"starts_at": "2025-06-15T08:00:00Z",
						"ends_at":   "2025-06-15T20:00:00Z",
					},
					"relationships": map[string]interface{}{
						"user": map[string]interface{}{
							"data": map[string]interface{}{"id": "user-1", "type": "users"},
						},
						"schedule": map[string]interface{}{
							"data": map[string]interface{}{"id": "sched-1", "type": "on_call_schedules"},
						},
					},
				},
			},
			"included": []map[string]interface{}{
				{
					"id":   "user-1",
					"type": "users",
					"attributes": map[string]interface{}{
						"name":  "Alice",
						"email": "alice@example.com",
					},
				},
				{
					"id":   "sched-1",
					"type": "on_call_schedules",
					"attributes": map[string]interface{}{
						"name": "Primary",
					},
				},
			},
			"meta": map[string]interface{}{
				"current_page": 1,
				"total_pages":  1,
				"total_count":  1,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	result, err := client.ListShiftsCLI(context.Background(), 1, 25, nil)
	if err != nil {
		t.Fatalf("ListShiftsCLI returned error: %v", err)
	}

	if len(result.Shifts) != 1 {
		t.Fatalf("expected 1 shift, got %d", len(result.Shifts))
	}
	if result.Shifts[0].UserName != "Alice" {
		t.Errorf("UserName = %q, want %q", result.Shifts[0].UserName, "Alice")
	}
	if result.Shifts[0].UserEmail != "alice@example.com" {
		t.Errorf("UserEmail = %q, want %q", result.Shifts[0].UserEmail, "alice@example.com")
	}
	if result.Shifts[0].ScheduleName != "Primary" {
		t.Errorf("ScheduleName = %q, want %q", result.Shifts[0].ScheduleName, "Primary")
	}
}

func TestListShiftsCLIUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.ListShiftsCLI(context.Background(), 1, 25, nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

// --- parseAlertData ---

func TestParseAlertData(t *testing.T) {
	desc := "CPU at 95%"
	status := "triggered"
	extURL := "https://datadog.com/alert/1"

	d := alertResponseData{
		ID: "alert-1",
	}
	d.Attributes.ShortID = "ALT-1A"
	d.Attributes.Summary = " High CPU "
	d.Attributes.Source = " datadog "
	d.Attributes.Description = &desc
	d.Attributes.Status = &status
	d.Attributes.ExternalURL = &extURL
	d.Attributes.CreatedAt = "2025-06-15T10:00:00Z"
	d.Attributes.UpdatedAt = "2025-06-15T11:00:00Z"
	d.Attributes.Services = []struct {
		Name string `json:"name"`
	}{{Name: "web"}}
	d.Attributes.Environments = []struct {
		Name string `json:"name"`
	}{{Name: "production"}}
	d.Attributes.Groups = []struct {
		Name string `json:"name"`
	}{{Name: "SRE"}}
	d.Attributes.Labels = flexibleLabels{{Key: "host", Value: "web-1"}}

	alert := parseAlertData(d)

	if alert.ShortID != "ALT-1A" {
		t.Errorf("ShortID = %q, want %q", alert.ShortID, "ALT-1A")
	}
	if alert.Summary != "High CPU" {
		t.Errorf("Summary = %q, want %q", alert.Summary, "High CPU")
	}
	if alert.Source != "datadog" {
		t.Errorf("Source = %q, want %q", alert.Source, "datadog")
	}
	if alert.Description != "CPU at 95%" {
		t.Errorf("Description = %q, want %q", alert.Description, "CPU at 95%")
	}
	if alert.Status != "triggered" {
		t.Errorf("Status = %q, want %q", alert.Status, "triggered")
	}
	if len(alert.Services) != 1 || alert.Services[0] != "web" {
		t.Errorf("Services = %v, want [web]", alert.Services)
	}
	if len(alert.Labels) != 1 || alert.Labels["host"] != "web-1" {
		t.Errorf("Labels = %v, want {host: web-1}", alert.Labels)
	}
}

func TestParseAlertDataMinimal(t *testing.T) {
	d := alertResponseData{ID: "alert-min"}
	d.Attributes.Summary = "Minimal"
	d.Attributes.Source = "manual"
	d.Attributes.CreatedAt = "2025-01-01T00:00:00Z"
	d.Attributes.UpdatedAt = "2025-01-01T00:00:00Z"

	alert := parseAlertData(d)

	if alert.ID != "alert-min" {
		t.Errorf("ID = %q, want %q", alert.ID, "alert-min")
	}
	if alert.Description != "" {
		t.Errorf("Description = %q, want empty", alert.Description)
	}
	if alert.Status != "" {
		t.Errorf("Status = %q, want empty", alert.Status)
	}
}

func TestFlexibleLabels_Array(t *testing.T) {
	input := `[{"key":"host","value":"web-1"},{"key":"count","value":42}]`
	var labels flexibleLabels
	if err := json.Unmarshal([]byte(input), &labels); err != nil {
		t.Fatalf("Unmarshal array: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("got %d labels, want 2", len(labels))
	}
	if labels[0].Key != "host" || labels[0].Value != "web-1" {
		t.Errorf("labels[0] = %+v, want {host web-1}", labels[0])
	}
}

func TestFlexibleLabels_EmptyObject(t *testing.T) {
	input := `{}`
	var labels flexibleLabels
	if err := json.Unmarshal([]byte(input), &labels); err != nil {
		t.Fatalf("Unmarshal empty object: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("got %d labels, want 0", len(labels))
	}
}

func TestFlexibleLabels_ObjectWithKeys(t *testing.T) {
	input := `{"env":"prod","region":"us-east-1"}`
	var labels flexibleLabels
	if err := json.Unmarshal([]byte(input), &labels); err != nil {
		t.Fatalf("Unmarshal object: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("got %d labels, want 2", len(labels))
	}
	m := make(map[string]interface{})
	for _, l := range labels {
		m[l.Key] = l.Value
	}
	if m["env"] != "prod" {
		t.Errorf("env = %v, want prod", m["env"])
	}
	if m["region"] != "us-east-1" {
		t.Errorf("region = %v, want us-east-1", m["region"])
	}
}
