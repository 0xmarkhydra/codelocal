import {interpolate, useCurrentFrame} from "remotion";

export const FlowPath: React.FC<{d: string; color: string; delay?: number; width?: number; opacity?: number}> = ({d, color, delay = 0, width = 5, opacity = 1}) => {
  const frame = useCurrentFrame();
  return (
    <>
      <path d={d} fill="none" stroke={color} strokeWidth={width + 10} opacity={0.1 * opacity} filter="blur(7px)" />
      <path d={d} fill="none" stroke={color} strokeWidth={width} strokeLinecap="round" opacity={opacity} strokeDasharray="18 28" strokeDashoffset={-interpolate(frame - delay, [0, 90], [0, 260], {extrapolateLeft: "clamp", extrapolateRight: "extend"})} />
    </>
  );
};
