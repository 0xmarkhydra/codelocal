import {TransitionSeries, linearTiming} from "@remotion/transitions";
import {fade} from "@remotion/transitions/fade";
import {AbsoluteFill} from "remotion";
import {Scene01Hook} from "./scenes/Scene01Hook";
import {Scene02Router} from "./scenes/Scene02Router";
import {Scene03Buffet} from "./scenes/Scene03Buffet";
import {Scene04Agent} from "./scenes/Scene04Agent";
import {Scene05NineRouter} from "./scenes/Scene05NineRouter";
import {Scene06OmniRoute} from "./scenes/Scene06OmniRoute";
import {Scene07Choice} from "./scenes/Scene07Choice";
import {Scene08Final} from "./scenes/Scene08Final";
import {BrandWatermark} from "./components/BrandWatermark";

const FadeTransition: React.FC = () => (
  <TransitionSeries.Transition
    presentation={fade()}
    timing={linearTiming({durationInFrames: 15})}
  />
);

export const AIRouterShort: React.FC = () => {
  return (
    <AbsoluteFill style={{backgroundColor: "#020712"}}>
      <TransitionSeries>
        <TransitionSeries.Sequence durationInFrames={165} name="01 • Hook">
          <Scene01Hook />
        </TransitionSeries.Sequence>
        <FadeTransition />
        <TransitionSeries.Sequence durationInFrames={225} name="02 • Một cổng chung">
          <Scene02Router />
        </TransitionSeries.Sequence>
        <FadeTransition />
        <TransitionSeries.Sequence durationInFrames={255} name="03 • Buffet và API">
          <Scene03Buffet />
        </TransitionSeries.Sequence>
        <FadeTransition />
        <TransitionSeries.Sequence durationInFrames={255} name="04 • Coding agent">
          <Scene04Agent />
        </TransitionSeries.Sequence>
        <FadeTransition />
        <TransitionSeries.Sequence durationInFrames={255} name="05 • 9Router">
          <Scene05NineRouter />
        </TransitionSeries.Sequence>
        <FadeTransition />
        <TransitionSeries.Sequence durationInFrames={255} name="06 • OmniRoute">
          <Scene06OmniRoute />
        </TransitionSeries.Sequence>
        <FadeTransition />
        <TransitionSeries.Sequence durationInFrames={285} name="07 • Chọn cái nào">
          <Scene07Choice />
        </TransitionSeries.Sequence>
        <FadeTransition />
        <TransitionSeries.Sequence durationInFrames={210} name="08 • Kết luận">
          <Scene08Final />
        </TransitionSeries.Sequence>
      </TransitionSeries>
      <BrandWatermark />
    </AbsoluteFill>
  );
};
