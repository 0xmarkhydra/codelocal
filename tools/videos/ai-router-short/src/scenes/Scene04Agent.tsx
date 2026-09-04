import {interpolate, useCurrentFrame} from "remotion";
import {GlassCard} from "../components/GlassCard";
import {SceneShell} from "../components/SceneShell";
import {BigTitle, Kicker} from "../components/Typography";
import {colors, safe} from "../theme";

const steps = [
  {label: "ĐỌC CODE", icon: "</>", color: colors.cyan},
  {label: "GỌI TOOL", icon: "◆", color: colors.gold},
  {label: "ĐỌC LOG", icon: "≡", color: "#B8A0FF"},
  {label: "THỬ LẠI", icon: "↻", color: colors.danger},
  {label: "TRẢ KẾT QUẢ", icon: "✓", color: colors.green},
];

export const Scene04Agent: React.FC = () => {
  const frame = useCurrentFrame();
  const tokenCount = Math.round(interpolate(frame, [20, 205], [12000, 248000], {extrapolateLeft: "clamp", extrapolateRight: "clamp"}));
  return (
    <SceneShell accent={colors.danger} secondary={colors.cyan}>
      <div style={{position: "absolute", left: safe.left, right: safe.right, top: 126, textAlign: "center"}}>
        <Kicker color={colors.danger}>VÌ SAO TỐN NHANH?</Kicker>
        <div style={{marginTop: 34}}>
          <BigTitle align="center" size={83} lines={[<>CODING AGENT</>, <><span style={{color: colors.danger}}>RẤT NGỐN TOKEN</span></>]} />
        </div>
      </div>

      <div style={{position: "absolute", left: 128, top: 570, width: 824, height: 900}}>
        <div style={{position: "absolute", left: 91, top: 80, bottom: 80, width: 5, borderRadius: 4, background: "rgba(255,255,255,.09)"}}>
          <div style={{width: "100%", height: `${interpolate(frame, [12, 132], [0, 100], {extrapolateLeft: "clamp", extrapolateRight: "clamp"})}%`, background: `linear-gradient(${colors.cyan}, ${colors.gold}, ${colors.danger}, ${colors.green})`, boxShadow: `0 0 20px ${colors.cyan}`}} />
        </div>
        {steps.map((step, index) => (
          <GlassCard key={step.label} accent={`${step.color}66`} start={16 + index * 15} style={{height: 135, marginBottom: 25, padding: "24px 34px 24px 130px"}}>
            <div style={{position: "absolute", left: 28, top: 22, width: 90, height: 90, borderRadius: 26, display: "grid", placeItems: "center", background: `${step.color}18`, border: `2px solid ${step.color}`, color: step.color, fontSize: 34, fontWeight: 900, boxShadow: `0 0 28px ${step.color}33`}}>{step.icon}</div>
            <div style={{fontSize: 35, fontWeight: 900, color: colors.text, marginTop: 4}}>{step.label}</div>
            <div style={{height: 10, borderRadius: 8, marginTop: 16, background: "rgba(255,255,255,.08)", overflow: "hidden"}}>
              <div style={{height: "100%", width: `${interpolate(frame, [25 + index * 12, 85 + index * 12], [0, 88 - index * 8], {extrapolateLeft: "clamp", extrapolateRight: "clamp"})}%`, borderRadius: 8, background: step.color, boxShadow: `0 0 18px ${step.color}`}} />
            </div>
          </GlassCard>
        ))}
      </div>

      <div style={{position: "absolute", left: safe.left, right: safe.right, bottom: 180}}>
        <GlassCard accent={`${colors.danger}88`} start={82} style={{padding: "34px 42px", display: "flex", alignItems: "center", justifyContent: "space-between"}}>
          <div>
            <div style={{fontSize: 25, color: colors.muted, letterSpacing: 4, fontWeight: 800}}>TOKEN ĐANG DÙNG</div>
            <div style={{fontSize: 64, fontWeight: 900, color: colors.danger, fontVariantNumeric: "tabular-nums"}}>{tokenCount.toLocaleString("vi-VN")}</div>
          </div>
          <div style={{textAlign: "right", fontSize: 31, lineHeight: 1.35, fontWeight: 750}}>
            Gọi nhiều vòng<br/><span style={{color: colors.danger}}>→ API dễ đội chi phí</span>
          </div>
        </GlassCard>
      </div>
    </SceneShell>
  );
};
