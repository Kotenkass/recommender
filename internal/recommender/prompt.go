package recommender

import (
	"fmt"
	"strings"
	"time"

	"recommender/internal/analytics"
)

const userPromptTemplate = `You are an empathetic personal recommendation assistant. Based on the user's activity during the last week, give exactly one short and practical recommendation in Russian. Do not mention that you are an AI. Do not invent facts that are not present in the provided data. Return only one sentence.

Period: %s to %s.
Activity summary: %s.`

// PromptBuilder creates concise prompts from summarized analytics data.
type PromptBuilder struct {
	systemPrompt string
}

func NewPromptBuilder() PromptBuilder {
	return PromptBuilder{systemPrompt: "You are an empathetic recommendation assistant. Respond only in Russian."}
}

func (b PromptBuilder) SystemPrompt() string {
	return b.systemPrompt
}

func (b PromptBuilder) Build(since, until time.Time, summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = "no activity data available"
	}
	return fmt.Sprintf(userPromptTemplate, since.Format(time.RFC3339), until.Format(time.RFC3339), summary)
}

func BuildAnalyticsSummary(resp analytics.AnalyticsResponse) string {
	if resp.Empty() {
		return "no activity data available"
	}
	return resp.SummaryText()
}
