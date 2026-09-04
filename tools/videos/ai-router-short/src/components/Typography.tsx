import {Easing, interpolate, spring, useCurrentFrame, useVideoConfig} from "remotion";
import {colors, titleShadow} from "../theme";

export const Kicker: React.FC<{children: React.ReactNode; color?: string}> = ({children, color = colors.cyan}) => {
  const frame = useCurrentFrame();
  return (
    <div style={{display: "inline-flex", alignItems: "center", gap: 12, color, fontSize: 26, fontWeight: 800, letterSpacing: 5, textTransform: "uppercase", opacity: interpolate(frame, [0, 12], [0, 1], {extrapolateLeft: "clamp", extrapolateRight: "clamp"})}}>
      <span style={{width: 44, height: 3, background: color, boxShadow: `0 0 16px ${color}`}} />
      {children}
    </div>
  );
};

export const BigTitle: React.FC<{lines: React.ReactNode[]; start?: number; size?: number; align?: "left" | "center"}> = ({lines, start = 4, size = 94, align = "left"}) => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  return (
    <div style={{display: "flex", flexDirection: "column", alignItems: align === "center" ? "center" : "flex-start"}}>
      {lines.map((line, index) => (
        <div
          key={index}
          style={{
            fontSize: size,
            fontWeight: 900,
            lineHeight: 1.04,
            letterSpacing: -3.2,
            textAlign: align,
            textShadow: titleShadow,
            opacity: interpolate(frame, [start + index * 7, start + index * 7 + 12], [0, 1], {extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: Easing.bezier(0.16, 1, 0.3, 1)}),
            translate: `0px ${interpolate(frame, [start + index * 7, start + index * 7 + 16], [38, 0], {extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: Easing.bezier(0.16, 1, 0.3, 1)})}px`,
            scale: spring({frame: frame - start - index * 7, fps, config: {damping: 18, stiffness: 150}}),
          }}
        >
          {line}
        </div>
      ))}
    </div>
  );
};

export const Caption: React.FC<{children: React.ReactNode; start?: number; color?: string}> = ({children, start = 20, color = colors.muted}) => {
  const frame = useCurrentFrame();
  return (
    <div style={{color, fontSize: 37, fontWeight: 600, lineHeight: 1.4, opacity: interpolate(frame, [start, start + 14], [0, 1], {extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: Easing.bezier(0.16, 1, 0.3, 1)}), translate: `0px ${interpolate(frame, [start, start + 14], [22, 0], {extrapolateLeft: "clamp", extrapolateRight: "clamp"})}px`}}>
      {children}
    </div>
  );
};
