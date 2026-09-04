import {Easing, interpolate, spring, useCurrentFrame, useVideoConfig} from "remotion";
import {FlowPath} from "../components/FlowSvg";
import {GlassCard} from "../components/GlassCard";
import {SceneShell} from "../components/SceneShell";
import {BigTitle, Kicker} from "../components/Typography";
import {colors, safe} from "../theme";

const providers = [
  {name: "GPT", x: 90, y: 105, color: "#80F5C8"},
  {name: "Claude", x: 90, y: 340, color: colors.goldSoft},
  {name: "Gemini", x: 730, y: 95, color: "#B8A0FF"},
  {name: "Kimi", x: 760, y: 335, color: colors.cyanSoft},
  {name: "GLM", x: 390, y: 510, color: "#FF85B1"},
];

export const Scene02Router: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  return (
    <SceneShell accent={colors.cyan} secondary={colors.gold}>
      <div style={{position: "absolute", left: safe.left, right: safe.right, top: 128, textAlign: "center"}}>
        <Kicker color={colors.cyan}>GIẢI THÍCH SIÊU DỄ</Kicker>
        <div style={{marginTop: 34}}>
          <BigTitle align="center" size={86} lines={[<>NHIỀU AI</>, <><span style={{color: colors.green}}>→ 1 CỔNG CHUNG</span></>]} />
        </div>
      </div>

      <div style={{position: "absolute", left: 80, top: 575, width: 920, height: 700}}>
        <svg viewBox="0 0 920 700" style={{position: "absolute", inset: 0}}>
          {providers.map((provider, index) => (
            <FlowPath key={provider.name} d={`M ${provider.x + 70} ${provider.y + 50} Q 460 ${230 + index * 24} 460 330`} color={provider.color} delay={index * 8} width={6} />
          ))}
          <FlowPath d="M 460 390 C 460 470 460 555 460 660" color={colors.green} delay={32} width={10} />
        </svg>

        {providers.map((provider, index) => (
          <div
            key={provider.name}
            style={{
              position: "absolute",
              left: provider.x,
              top: provider.y,
              width: 150,
              height: 98,
              borderRadius: 24,
              display: "grid",
              placeItems: "center",
              background: "rgba(7,17,33,.9)",
              border: `2px solid ${provider.color}`,
              boxShadow: `0 0 32px ${provider.color}44`,
              fontSize: 27,
              fontWeight: 800,
              opacity: interpolate(frame, [10 + index * 6, 24 + index * 6], [0, 1], {extrapolateLeft: "clamp", extrapolateRight: "clamp"}),
              translate: `0px ${interpolate(frame, [10 + index * 6, 24 + index * 6], [-60, 0], {extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: Easing.bezier(0.16, 1, 0.3, 1)})}px`,
            }}
          >
            {provider.name}
          </div>
        ))}

        <div
          style={{
            position: "absolute",
            left: 325,
            top: 190,
            width: 270,
            height: 270,
            borderRadius: "50%",
            display: "grid",
            placeItems: "center",
            background: "radial-gradient(circle at 35% 25%, rgba(255,255,255,.5), rgba(38,226,255,.24) 28%, rgba(4,16,35,.95) 66%)",
            border: "2px solid rgba(151,239,255,.72)",
            boxShadow: `0 0 ${60 + Math.sin(frame / 8) * 16}px rgba(38,226,255,.38), inset 0 0 50px rgba(38,226,255,.16)`,
            scale: spring({frame: frame - 28, fps, config: {damping: 16, stiffness: 120}}),
          }}
        >
          <div style={{textAlign: "center"}}>
            <div style={{fontSize: 25, color: colors.cyanSoft, letterSpacing: 5, fontWeight: 700}}>AI</div>
            <div style={{fontSize: 43, fontWeight: 900}}>ROUTER</div>
          </div>
        </div>
        <div style={{position: "absolute", left: 325, top: 626, width: 270, height: 64, borderRadius: 32, display: "grid", placeItems: "center", background: colors.green, color: "#032013", fontSize: 25, fontWeight: 900, boxShadow: `0 0 32px ${colors.green}88`}}>1 CỔNG RA</div>
      </div>

      <div style={{position: "absolute", left: safe.left, right: safe.right, top: 1360, display: "grid", gap: 18}}>
        <GlassCard accent={`${colors.gold}66`} start={58} style={{padding: "28px 34px"}}>
          <div style={{display: "flex", alignItems: "center", gap: 22, fontSize: 34, fontWeight: 750}}>
            <span style={{color: colors.gold}}>GPT hết giới hạn?</span><span style={{color: colors.muted}}>→</span><span style={{color: colors.text}}>Tự chuyển Claude.</span>
          </div>
        </GlassCard>
        <GlassCard accent={`${colors.cyan}66`} start={76} style={{padding: "28px 34px"}}>
          <div style={{display: "flex", alignItems: "center", gap: 22, fontSize: 34, fontWeight: 750}}>
            <span style={{color: colors.cyan}}>Claude lỗi?</span><span style={{color: colors.muted}}>→</span><span style={{color: colors.text}}>Tự chuyển Gemini.</span>
          </div>
        </GlassCard>
      </div>
    </SceneShell>
  );
};
