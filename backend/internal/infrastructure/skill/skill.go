// Package skill defines the Skill interface for extending agent capabilities.
// Skills are self-contained capabilities that agents can invoke to perform
// specific tasks (e.g., stock price queries, web search, crypto data).
//
// To add a new skill:
//  1. Implement the Skill interface
//  2. Register it via Registry.Register()
//  3. Inject the registry into the agent that needs it
package skill

import "context"

// SkillParam describes a single input parameter for a skill.
// The Type field follows JSON Schema conventions: "string", "number", "integer", "boolean".
type SkillParam struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Enum        []string `json:"enum,omitempty"`
}

// Skill represents a self-contained capability that an agent can use.
type Skill interface {
	// Name returns the unique identifier of this skill (used as the function name in LLM tool calls).
	Name() string
	// Description returns a human-readable description (used in LLM tool definitions).
	Description() string
	// Parameters returns the input parameter schema for this skill.
	Parameters() []SkillParam
	// Execute runs the skill with the given input parameters.
	Execute(ctx context.Context, input map[string]interface{}) (interface{}, error)
}

// ParametersSchema converts a []SkillParam into an OpenAI-compatible JSON Schema object.
func ParametersSchema(params []SkillParam) map[string]interface{} {
	properties := make(map[string]interface{})
	required := []string{}
	for _, p := range params {
		prop := map[string]interface{}{
			"type":        p.Type,
			"description": p.Description,
		}
		if len(p.Enum) > 0 {
			prop["enum"] = p.Enum
		}
		properties[p.Name] = prop
		if p.Required {
			required = append(required, p.Name)
		}
	}
	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// Registry holds all registered skills.
type Registry struct {
	skills map[string]Skill
}

// NewRegistry creates an empty skill registry.
func NewRegistry() *Registry {
	return &Registry{skills: make(map[string]Skill)}
}

// Register adds a skill to the registry. Panics on duplicate name.
func (r *Registry) Register(s Skill) {
	if _, exists := r.skills[s.Name()]; exists {
		panic("skill already registered: " + s.Name())
	}
	r.skills[s.Name()] = s
}

// Get retrieves a skill by name.
func (r *Registry) Get(name string) (Skill, bool) {
	s, ok := r.skills[name]
	return s, ok
}

// Count returns the number of registered skills.
func (r *Registry) Count() int {
	return len(r.skills)
}

// List returns metadata for all registered skills.
func (r *Registry) List() []SkillMeta {
	result := make([]SkillMeta, 0, len(r.skills))
	for _, s := range r.skills {
		result = append(result, SkillMeta{
			Name:        s.Name(),
			Description: s.Description(),
			Parameters:  s.Parameters(),
		})
	}
	return result
}

// SkillMeta holds metadata about a skill (used for LLM tool definitions).
type SkillMeta struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Parameters  []SkillParam `json:"parameters"`
}
