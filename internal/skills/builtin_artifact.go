package skills

// BuiltinArtifacts materializes the currently promoted built-in manifests into
// immutable artifacts using the bounded seed knowledge compiled with CodeLocal.
// Cloud/Desktop runtimes use this only as a safe fallback when no promoted full
// package is available from the durable package registry.
func BuiltinArtifacts(registry *Registry) ([]Artifact, error) {
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
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

// DefaultArtifactKnowledgeStore keeps the default engine on the same immutable
// artifact contract used by Cloud/Desktop while retaining a bounded fallback
// when durable promoted packages are unavailable.
func DefaultArtifactKnowledgeStore(registry *Registry) KnowledgeStore {
	if registry == nil {
		registry = DefaultRegistry()
	}
	artifacts, err := BuiltinArtifacts(registry)
	if err != nil {
		panic(err)
	}
	store, err := NewArtifactKnowledgeStore(registry, artifacts...)
	if err != nil {
		panic(err)
	}
	return store
}
