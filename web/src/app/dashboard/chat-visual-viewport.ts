export type ChatViewportFrame = {
  height: number;
  offsetTop: number;
};

export function chatViewportFrame(visualViewport: Pick<VisualViewport, "height" | "offsetTop"> | null, innerHeight: number): ChatViewportFrame {
  return {
    height: Math.max(1, Math.round(visualViewport?.height ?? innerHeight)),
    offsetTop: Math.max(0, Math.round(visualViewport?.offsetTop ?? 0)),
  };
}
