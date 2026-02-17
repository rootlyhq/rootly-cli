package api

import "time"

// MockIncidents returns sample incident data for testing
func MockIncidents() []Incident {
	now := time.Now()
	hourAgo := now.Add(-1 * time.Hour)
	twoHoursAgo := now.Add(-2 * time.Hour)
	dayAgo := now.Add(-24 * time.Hour)

	return []Incident{
		{
			ID:              "inc_001",
			SequentialID:    "INC-142",
			Title:           "Database Connection Failure",
			Summary:         "Production database is experiencing connection timeouts",
			Status:          "in_progress",
			Severity:        "critical",
			Kind:            "incident",
			CreatedAt:       twoHoursAgo,
			StartedAt:       &twoHoursAgo,
			DetectedAt:      &hourAgo,
			AcknowledgedAt:  &hourAgo,
			Services:        []string{"api", "database", "web"},
			Environments:    []string{"production"},
			Teams:           []string{"Platform", "SRE"},
			SlackChannelURL: "https://slack.com/archives/C123456",
			JiraIssueURL:    "https://jira.example.com/browse/INC-123",
		},
		{
			ID:           "inc_002",
			SequentialID: "INC-141",
			Title:        "High API Latency",
			Summary:      "API response times increased by 300%",
			Status:       "acknowledged",
			Severity:     "high",
			Kind:         "incident",
			CreatedAt:    hourAgo,
			StartedAt:    &hourAgo,
			DetectedAt:   &hourAgo,
			Services:     []string{"api", "gateway"},
			Environments: []string{"production"},
			Teams:        []string{"Backend"},
		},
		{
			ID:           "inc_003",
			SequentialID: "INC-140",
			Title:        "Deployment Pipeline Failed",
			Summary:      "CI/CD pipeline failing for main branch",
			Status:       "resolved",
			Severity:     "medium",
			Kind:         "incident",
			CreatedAt:    dayAgo,
			StartedAt:    &dayAgo,
			ResolvedAt:   &hourAgo,
			Services:     []string{"ci-cd"},
			Environments: []string{"staging"},
			Teams:        []string{"DevOps"},
		},
		{
			ID:           "inc_004",
			SequentialID: "INC-139",
			Title:        "Disk Space Warning",
			Summary:      "Log volume approaching capacity on worker nodes",
			Status:       "mitigated",
			Severity:     "low",
			Kind:         "incident",
			CreatedAt:    dayAgo,
			MitigatedAt:  &hourAgo,
			Services:     []string{"workers", "logging"},
			Environments: []string{"production", "staging"},
			Teams:        []string{"Infrastructure"},
		},
		{
			ID:           "inc_005",
			SequentialID: "INC-143",
			Title:        "Authentication Service Degraded",
			Summary:      "OAuth token refresh failing intermittently",
			Status:       "started",
			Severity:     "high",
			Kind:         "incident",
			CreatedAt:    now,
			StartedAt:    &now,
			Services:     []string{"auth", "oauth"},
			Environments: []string{"production"},
			Teams:        []string{"Security", "Platform"},
		},
	}
}

// MockAlerts returns sample alert data for testing
func MockAlerts() []Alert {
	now := time.Now()
	hourAgo := now.Add(-1 * time.Hour)
	twoHoursAgo := now.Add(-2 * time.Hour)
	dayAgo := now.Add(-24 * time.Hour)

	return []Alert{
		{
			ID:           "alert_001",
			ShortID:      "ALT-8F2A",
			Summary:      "High CPU Usage on web-prod-1",
			Description:  "CPU utilization has exceeded 90% for more than 5 minutes",
			Status:       "triggered",
			Source:       "datadog",
			CreatedAt:    now,
			StartedAt:    &now,
			ExternalURL:  "https://app.datadoghq.com/monitors/123456",
			Services:     []string{"web"},
			Environments: []string{"production"},
			Groups:       []string{"Infrastructure"},
			Labels: map[string]string{
				"severity": "warning",
				"host":     "web-prod-1",
			},
		},
		{
			ID:           "alert_002",
			ShortID:      "ALT-7B1C",
			Summary:      "Database Replica Lag > 30s",
			Description:  "Replication lag on db-replica-2 has exceeded threshold",
			Status:       "acknowledged",
			Source:       "grafana",
			CreatedAt:    hourAgo,
			StartedAt:    &hourAgo,
			ExternalURL:  "https://grafana.example.com/d/abc123",
			Services:     []string{"database"},
			Environments: []string{"production"},
			Groups:       []string{"Database"},
			Labels: map[string]string{
				"replica": "db-replica-2",
				"region":  "us-east-1",
			},
		},
		{
			ID:           "alert_003",
			ShortID:      "ALT-6D9E",
			Summary:      "SSL Certificate Expiring Soon",
			Description:  "Certificate for api.example.com expires in 7 days",
			Status:       "open",
			Source:       "pagerduty",
			CreatedAt:    dayAgo,
			ExternalURL:  "https://example.pagerduty.com/incidents/P123",
			Services:     []string{"api"},
			Environments: []string{"production"},
			Groups:       []string{"Security"},
			Labels: map[string]string{
				"domain":         "api.example.com",
				"days_to_expiry": "7",
			},
		},
		{
			ID:           "alert_004",
			ShortID:      "ALT-5E3F",
			Summary:      "Memory Usage Critical on worker-3",
			Description:  "Memory usage at 95%, potential OOM risk",
			Status:       "resolved",
			Source:       "datadog",
			CreatedAt:    twoHoursAgo,
			StartedAt:    &twoHoursAgo,
			EndedAt:      &hourAgo,
			ExternalURL:  "https://app.datadoghq.com/monitors/789",
			Services:     []string{"workers"},
			Environments: []string{"production"},
			Groups:       []string{"Infrastructure"},
			Labels: map[string]string{
				"host": "worker-3",
			},
		},
		{
			ID:           "alert_005",
			ShortID:      "ALT-9A4B",
			Summary:      "Error Rate Spike in Payment Service",
			Description:  "5xx error rate increased to 5% in the last 10 minutes",
			Status:       "triggered",
			Source:       "grafana",
			CreatedAt:    now,
			StartedAt:    &now,
			ExternalURL:  "https://grafana.example.com/d/payments",
			Services:     []string{"payments", "checkout"},
			Environments: []string{"production"},
			Groups:       []string{"Payments"},
			Labels: map[string]string{
				"error_rate": "5%",
				"threshold":  "1%",
			},
		},
		{
			ID:           "alert_006",
			ShortID:      "ALT-2C7D",
			Summary:      "Kubernetes Pod CrashLoopBackOff",
			Description:  "Pod api-deployment-abc123 is in CrashLoopBackOff state",
			Status:       "triggered",
			Source:       "slack",
			CreatedAt:    hourAgo,
			StartedAt:    &hourAgo,
			Services:     []string{"api"},
			Environments: []string{"staging"},
			Groups:       []string{"Platform"},
			Labels: map[string]string{
				"pod":       "api-deployment-abc123",
				"namespace": "default",
			},
		},
	}
}
