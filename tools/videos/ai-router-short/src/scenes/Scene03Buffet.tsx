import {interpolate, useCurrentFrame} from "remotion";
import {GlassCard} from "../components/GlassCard";
import {SceneShell} from "../components/SceneShell";
import {BigTitle, Kicker} from "../components/Typography";
import {colors, safe} from "../theme";

export const Scene03Buffet: React.FC = () => {
  const frame = useCurrentFrame();
  const meter = Math.round(interpolate(frame, [25, 210], [8, 94], {extrapolateLeft: "clamp", extrapolateRight: "clamp"}));
  return (
    <SceneShell accent={colors.green} secondary={colors.danger}>
      <div style={{position: "absolute", left: safe.left, right: safe.right, top: 126, textAlign: "center"}}>
        <Kicker color={colors.green}>INSIGHT QUAN TRỌNG</Kicker>
        <div style={{marginTop: 35}}>
          <BigTitle align="center" size={78} lines={[<><span style={{color: colors.green}}>GÓI THÁNG</span> = BUFFET</>, <><span style={{color: colors.danger}}>API</span> = TÍNH THEO MỨC DÙNG</>]} />
        </div>
      </div>

      <div style={{position: "absolute", left: 64, right: 64, top: 620, display: "flex", gap: 28}}>
        <GlassCard accent={`${colors.green}77`} start={12} style={{width: 462, height: 655, padding: 38}}>
          <div style={{fontSize: 29, color: colors.green, fontWeight: 900, letterSpacing: 3}}>GÓI THÁNG</div>
          <div style={{fontSize: 55, fontWeight: 900, marginTop: 14}}>BUFFET</div>
          <div style={{position: "relative", height: 340, marginTop: 26}}>
            <div style={{position: "absolute", left: 48, top: 92, width: 292, height: 154, borderRadius: "0 0 160px 160px", border: `10px solid ${colors.green}`, borderTop: 0, boxShadow: `0 20px 55px ${colors.green}33, inset 0 -28px 50px ${colors.green}22`}} />
            {[{x:72,y:70,c:colors.gold},{x:160,y:28,c:colors.cyan},{x:250,y:78,c:"#FF85B1"},{x:124,y:130,c:colors.green},{x:226,y:142,c:colors.goldSoft}].map((item,index)=>(
              <div key={index} style={{position:"absolute", left:item.x, top:item.y + Math.sin((frame + index * 13) / 13) * 8, width:66, height:66, borderRadius:20, background:item.c, boxShadow:`0 0 24px ${item.c}66`, rotate:`${index * 9 - 15}deg`}} />
            ))}
          </div>
          <div style={{display: "inline-flex", padding: "16px 22px", borderRadius: 28, color: "#042316", background: colors.green, fontSize: 25, fontWeight: 900}}>TRONG QUOTA THÁNG</div>
        </GlassCard>

        <GlassCard accent={`${colors.danger}77`} start={22} style={{width: 462, height: 655, padding: 38}}>
          <div style={{fontSize: 29, color: colors.danger, fontWeight: 900, letterSpacing: 3}}>API</div>
          <div style={{fontSize: 55, fontWeight: 900, marginTop: 14}}>ĐỒNG HỒ</div>
          <div style={{marginTop: 42, height: 305, display: "flex", alignItems: "center", justifyContent: "center"}}>
            <div style={{position: "relative", width: 270, height: 270, borderRadius: "50%", background: `conic-gradient(${colors.danger} ${meter}%, rgba(255,255,255,.08) ${meter}% 100%)`, boxShadow: `0 0 42px ${colors.danger}44`}}>
              <div style={{position: "absolute", inset: 24, borderRadius: "50%", background: "#071223", display: "grid", placeItems: "center", textAlign: "center"}}>
                <div><div style={{fontSize: 28, color: colors.muted}}>MỨC DÙNG</div><div style={{fontSize: 68, fontWeight: 900, color: colors.danger}}>{meter}%</div></div>
              </div>
            </div>
          </div>
          <div style={{fontSize: 27, color: colors.muted, fontWeight: 700}}>Dùng càng nhiều</div>
          <div style={{fontSize: 31, color: colors.text, fontWeight: 900, marginTop: 9}}>HÓA ĐƠN CÀNG TĂNG</div>
        </GlassCard>
      </div>

      <div style={{position: "absolute", left: safe.left, right: safe.right, top: 1352}}>
        <GlassCard accent={`${colors.green}88`} start={52} style={{padding: "38px 44px", textAlign: "center", boxShadow: `0 30px 100px rgba(0,0,0,.5), 0 0 60px ${colors.green}22`}}>
          <div style={{fontSize: 30, color: colors.muted, fontWeight: 650}}>Dùng nhiều nhưng vẫn trong quota</div>
          <div style={{fontSize: 45, color: colors.green, fontWeight: 900, marginTop: 16}}>GÓI THÁNG CÓ THỂ RẺ HƠN RẤT NHIỀU</div>
        </GlassCard>
      </div>
    </SceneShell>
  );
};
