import {Easing, interpolate, spring, useCurrentFrame, useVideoConfig} from "remotion";
import {SceneShell} from "../components/SceneShell";
import {BigTitle, Caption, Kicker} from "../components/Typography";
import {colors, glass, safe} from "../theme";

export const Scene01Hook: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const cardScale = spring({frame: frame - 32, fps, config: {damping: 14, stiffness: 150}});

  return (
    <SceneShell accent={colors.gold} secondary={colors.cyan}>
      <div style={{position: "absolute", inset: 0, display: "flex"}}>
        <div style={{width: "50%", background: "linear-gradient(180deg, rgba(255,198,45,.04), rgba(255,198,45,.14), transparent 78%)"}} />
        <div style={{width: "50%", background: "linear-gradient(180deg, rgba(38,226,255,.04), rgba(38,226,255,.14), transparent 78%)"}} />
      </div>

      <div style={{position: "absolute", left: safe.left, right: safe.right, top: 124, textAlign: "center"}}>
        <Kicker color={colors.danger}>MỞ MÀN</Kicker>
        <div style={{marginTop: 34}}>
          <BigTitle
            align="center"
            size={88}
            lines={[
              <>DÂN IT <span style={{color: colors.green}}>2026</span></>,
              <>CHƯA BIẾT</>,
              <><span style={{color: colors.gold}}>9Router</span> &amp; <span style={{color: colors.cyan}}>OmniRoute?</span></>,
            ]}
          />
        </div>
      </div>

      <div style={{position: "absolute", left: 78, right: 78, top: 765, height: 610}}>
        <div
          style={{
            ...glass,
            position: "absolute",
            left: 0,
            width: 430,
            height: 560,
            borderRadius: 42,
            borderColor: "rgba(255,198,45,.48)",
            boxShadow: "0 30px 90px rgba(0,0,0,.45), 0 0 65px rgba(255,198,45,.14)",
            padding: 42,
            opacity: interpolate(frame, [18, 38], [0, 1], {extrapolateLeft: "clamp", extrapolateRight: "clamp"}),
            translate: `${interpolate(frame, [18, 40], [-180, 0], {extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: Easing.bezier(0.16, 1, 0.3, 1)})}px 0px`,
            scale: cardScale,
          }}
        >
          <div style={{fontSize: 28, color: colors.goldSoft, letterSpacing: 4, fontWeight: 800}}>GỌN • NHANH</div>
          <div style={{fontSize: 65, fontWeight: 900, marginTop: 18, color: colors.gold}}>9Router</div>
          <div style={{position: "absolute", left: 46, right: 46, bottom: 60, height: 230}}>
            {[0, 1, 2].map((index) => (
              <div key={index} style={{position: "absolute", top: 25 + index * 66, left: index * 42, right: index * 28, height: 22, borderRadius: 14, background: `linear-gradient(90deg, ${colors.gold}, rgba(255,198,45,.08))`, opacity: 0.9 - index * 0.18, boxShadow: `0 0 22px ${colors.gold}`}} />
            ))}
          </div>
        </div>

        <div
          style={{
            ...glass,
            position: "absolute",
            right: 0,
            width: 430,
            height: 560,
            borderRadius: 42,
            borderColor: "rgba(38,226,255,.48)",
            boxShadow: "0 30px 90px rgba(0,0,0,.45), 0 0 65px rgba(38,226,255,.14)",
            padding: 42,
            opacity: interpolate(frame, [18, 38], [0, 1], {extrapolateLeft: "clamp", extrapolateRight: "clamp"}),
            translate: `${interpolate(frame, [18, 40], [180, 0], {extrapolateLeft: "clamp", extrapolateRight: "clamp", easing: Easing.bezier(0.16, 1, 0.3, 1)})}px 0px`,
            scale: cardScale,
          }}
        >
          <div style={{fontSize: 28, color: colors.cyanSoft, letterSpacing: 4, fontWeight: 800}}>RỘNG • SÂU</div>
          <div style={{fontSize: 59, fontWeight: 900, marginTop: 18, color: colors.cyan}}>OmniRoute</div>
          <svg viewBox="0 0 320 250" style={{position: "absolute", left: 52, right: 52, bottom: 42, width: 326}}>
            {[[40, 62], [150, 30], [270, 70], [72, 178], [235, 190]].map(([x, y], index) => (
              <g key={index}>
                <line x1="160" y1="120" x2={x} y2={y} stroke={colors.cyan} strokeWidth="4" opacity=".6" />
                <circle cx={x} cy={y} r="17" fill="#08182A" stroke={colors.cyan} strokeWidth="4" />
              </g>
            ))}
            <circle cx="160" cy="120" r="38" fill={colors.cyan} opacity=".22" />
            <circle cx="160" cy="120" r="25" fill={colors.cyan} />
          </svg>
        </div>

        <div
          style={{
            position: "absolute",
            left: "50%",
            top: 214,
            width: 132,
            height: 132,
            marginLeft: -66,
            borderRadius: "50%",
            display: "grid",
            placeItems: "center",
            background: "radial-gradient(circle, white 0%, #DDEEFF 12%, #14233F 36%, #050A13 70%)",
            border: "2px solid rgba(255,255,255,.7)",
            fontSize: 45,
            fontWeight: 900,
            boxShadow: `0 0 ${42 + Math.sin(frame / 5) * 12}px rgba(255,255,255,.75)`,
            opacity: interpolate(frame, [34, 46], [0, 1], {extrapolateLeft: "clamp", extrapolateRight: "clamp"}),
            scale: spring({frame: frame - 34, fps, config: {damping: 10, stiffness: 210}}),
          }}
        >
          VS
        </div>
      </div>

      <div style={{position: "absolute", left: safe.left, right: safe.right, top: 1460, textAlign: "center"}}>
        <Caption start={52} color={colors.text}>Có khi đang trả API hơi phí đấy 😏</Caption>
      </div>
    </SceneShell>
  );
};
