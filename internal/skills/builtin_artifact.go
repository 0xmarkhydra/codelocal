package skills

// DefaultArtifactKnowledgeStore materializes built-in seed knowledge through
// the same immutable artifact contract used by Cloud/Desktop caches. Keeping
// the production engine on this path prevents a second, memory-only knowledge
// format from becoming a hidden source of truth.
func DefaultArtifactKnowledgeStore(registry *Registry) KnowledgeStore {
	if registry == nil {
		registry = DefaultRegistry()
	}
	seed := BuiltinKnowledge()
	artifacts := make([]Artifact, 0, len(registry.List()))
	for _, manifest := range registry.List() {
		chunks := make([]KnowledgeChunk, 0)
		for _, chunk := range seed {
			if chunk.SkillID == manifest.ID && chunk.SkillVersion == manifest.Version {
				chunks = append(chunks, chunk)
			}
		}
		if len(chunks) == 0 {
			continue
		}
		artifact, err := BuildArtifact(manifest, chunks)
		if err != nil {
			panic(err)
		}
		artifacts = append(artifacts, artifact)
	}
	store, err := NewArtifactKnowledgeStore(registry, artifacts...)
	if err != nil {
		panic(err)
	}
	return store
}
