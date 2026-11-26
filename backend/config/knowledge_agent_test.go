package config

import "testing"

func TestRAGCredentialEnvironmentOverride(t *testing.T) {
	t.Setenv("RAG_CT_RAG_BASE_URL", "http://local-rag:5050")
	t.Setenv("RAG_CT_RAG_API_KEY", "test-service-key")
	c := &Config{}
	overrideWithEnv(c)
	if c.RAG.CTRAG.BaseURL != "http://local-rag:5050" || c.RAG.CTRAG.APIKey != "test-service-key" {
		t.Fatal("RAG service environment configuration not applied")
	}
}
