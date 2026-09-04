import {AbsoluteFill, interpolate, random, useCurrentFrame} from "remotion";
import {colors} from "../theme";

const particles = Array.from({length: 28}, (_, index) => ({
  x: random(`x-${index}`) * 100,
  y: random(`y-${index}`) * 100,
  size: 2 + random(`s-${index}`) * 4,
  delay: random(`d-${index}`) * 100,
}));

export const SceneShell: React.FC<{
  children: React.ReactNode;
  accent?: string;
  secondary?: string;
}> = ({children, accent = colors.cyan, secondary = colors.gold}) => {
  const frame = useCurrentFrame();
  return (
    <AbsoluteFill
      style={{
        overflow: "hidden",
        background: "radial-gradient(circle at 50% -10%, #10203D 0%, #061021 34%, #020712 72%)",
        color: colors.text,
      }}
    >
      <AbsoluteFill
        style={{
          opacity: 0.24,
          backgroundImage: "linear-gradient(rgba(123,174,217,.16) 1px, transparent 1px), linear-gradient(90deg, rgba(123,174,217,.16) 1px, transparent 1px)",
          backgroundSize: "72px 72px",
          translate: `0px ${interpolate(frame % 72, [0, 71], [0, 72])}px`,
          maskImage: "linear-gradient(to bottom, transparent, black 18%, black 82%, transparent)",
        }}
      />
      <div style={{position: "absolute", width: 780, height: 780, borderRadius: "50%", left: -360, top: 210, background: accent, filter: "blur(180px)", opacity: 0.12 + Math.sin(frame / 28) * 0.025}} />
      <div style={{position: "absolute", width: 700, height: 700, borderRadius: "50%", right: -360, bottom: 140, background: secondary, filter: "blur(190px)", opacity: 0.1 + Math.cos(frame / 31) * 0.025}} />
      {particles.map((particle, index) => (
        <div
          key={index}
          style={{
            position: "absolute",
            left: `${particle.x}%`,
            top: `${particle.y}%`,
            width: particle.size,
            height: particle.size,
            borderRadius: "50%",
            background: index % 2 === 0 ? accent : secondary,
            boxShadow: `0 0 18px ${index % 2 === 0 ? accent : secondary}`,
            opacity: 0.2 + 0.45 * ((Math.sin((frame + particle.delay) / 17) + 1) / 2),
            translate: `0px ${Math.sin((frame + particle.delay) / 25) * 12}px`,
          }}
        />
      ))}
      {children}
      <AbsoluteFill style={{pointerEvents: "none", boxShadow: "inset 0 0 190px rgba(0,0,0,.62)"}} />
    </AbsoluteFill>
  );
};
