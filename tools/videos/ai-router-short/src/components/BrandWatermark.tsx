import {useCurrentFrame} from "remotion";

export const BrandWatermark: React.FC = () => {
  const frame = useCurrentFrame();
  return (
    <div
      style={{
        position: "absolute",
        zIndex: 100,
        bottom: 50,
        left: 0,
        right: 0,
        textAlign: "center",
        color: "rgba(221, 235, 248, .58)",
        fontSize: 22,
        fontWeight: 600,
        letterSpacing: 4.5,
        opacity: 0.7 + Math.sin(frame / 24) * 0.08,
      }}
    >
      MMON • Cố Vấn AI
    </div>
  );
};
