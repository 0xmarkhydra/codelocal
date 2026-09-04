import {Easing, interpolate, useCurrentFrame} from "remotion";
import {glass} from "../theme";

export const GlassCard: React.FC<{children: React.ReactNode; accent?: string; start?: number; style?: React.CSSProperties}> = ({children, accent = "rgba(255,255,255,.16)", start = 0, style}) => {
  const frame = useCurrentFrame();
  return (
    <div style={{...glass, borderColor: accent, borderRadius: 34, position: "relative", overflow: "hidden", opacity: interpolate(frame, [start, start + 14], [0, 1], {extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: Easing.bezier(0.16, 1, 0.3, 1)}), translate: `0px ${interpolate(frame, [start, start + 18], [46, 0], {extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: Easing.bezier(0.16, 1, 0.3, 1)})}px`, ...style}}>
      <div style={{position: "absolute", inset: 0, background: `linear-gradient(120deg, transparent 0%, ${accent} 48%, transparent 62%)`, opacity: 0.16, translate: `${interpolate(frame % 120, [0, 119], [-900, 900])}px 0px`}} />
      <div style={{position: "relative", zIndex: 1}}>{children}</div>
    </div>
  );
};
