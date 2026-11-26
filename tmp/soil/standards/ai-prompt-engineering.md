
# Prompt Engineering Standards

## Prompt Structure

Every prompt should follow this hierarchy:

```
1. System prompt (role, constraints, output format)
2. Context (retrieved data, conversation history)
3. Few-shot examples (if applicable)
4. User instruction (the actual task)
5. Output format reminder (reinforce at the end)
```

## System Prompts

- Define the role clearly: "You are an incident response analyst at Rootly..."
- State constraints explicitly: what the agent should NOT do
- Specify output format upfront: JSON schema, markdown structure, etc.
- Include guardrails: "If you don't have enough information, say so. Do not fabricate."
- Keep system prompts stable — version them, don't edit in-flight

## Few-Shot Examples

- Use 2-3 examples for new/complex tasks, 0-1 for straightforward tasks
- Examples should cover: typical case, edge case, and "refusal" case (when to say no/I don't know)
- Use real data (anonymized) over synthetic data — models learn format AND judgment
- Keep examples concise — long examples waste tokens and can confuse

## Template Management

- Store prompts as templates (ERB, or string interpolation), not inline strings
- Separate prompt logic from business logic — prompts are configuration, not code
- Version prompts: when you change a prompt, you should be able to A/B test or roll back
- Parameterize dynamic content: `<%= incident.title %>`, not string concatenation
- Never interpolate raw user input into system prompts — always place in user message section

## Output Parsing

- Prefer structured output mode (JSON, tool use) over regex parsing of free text
- Always validate parsed output against expected schema
- Handle partial/malformed output: retry once with stricter format instructions, then fallback
- Log parse failures for prompt improvement — these are signal, not noise

## Guardrails

### Input
- Sanitize user input before including in prompts (strip injection attempts)
- Set max input length — truncate gracefully with notice
- Classify input intent before routing to expensive prompts

### Output
- Validate factual claims against source context (for RAG)
- Check for PII in output before returning to user
- Apply content filters appropriate to the use case
- Set max output tokens to prevent runaway generation

## Anti-Patterns

- **Prompt stuffing**: Cramming too many instructions — model attention degrades. Keep focused.
- **Implicit instructions**: Assuming the model "knows" your conventions. Be explicit.
- **Temperature 1.0 for factual tasks**: Use low temperature for deterministic tasks.
- **No examples for complex formats**: If output format is non-obvious, show don't tell.
- **Hardcoded prompts in code**: Makes iteration slow. Use templates with hot-reload in dev.
