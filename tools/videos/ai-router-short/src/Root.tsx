import "./index.css";
import {Composition, Folder} from "remotion";
import {AIRouterShort} from "./Composition";
import {Scene01Hook} from "./scenes/Scene01Hook";
import {Scene02Router} from "./scenes/Scene02Router";
import {Scene03Buffet} from "./scenes/Scene03Buffet";
import {Scene04Agent} from "./scenes/Scene04Agent";
import {Scene05NineRouter} from "./scenes/Scene05NineRouter";
import {Scene06OmniRoute} from "./scenes/Scene06OmniRoute";
import {Scene07Choice} from "./scenes/Scene07Choice";
import {Scene08Final} from "./scenes/Scene08Final";

const sceneDefaults = {fps: 30, width: 1080, height: 1920};

export const RemotionRoot: React.FC = () => {
  return (
    <>
      <Composition
        id="AIRouterShort"
        component={AIRouterShort}
        durationInFrames={1800}
        fps={30}
        width={1080}
        height={1920}
      />
      <Folder name="Scene previews">
        <Composition id="Scene01Hook" component={Scene01Hook} durationInFrames={150} {...sceneDefaults} />
        <Composition id="Scene02Router" component={Scene02Router} durationInFrames={210} {...sceneDefaults} />
        <Composition id="Scene03Buffet" component={Scene03Buffet} durationInFrames={240} {...sceneDefaults} />
        <Composition id="Scene04Agent" component={Scene04Agent} durationInFrames={240} {...sceneDefaults} />
        <Composition id="Scene05NineRouter" component={Scene05NineRouter} durationInFrames={240} {...sceneDefaults} />
        <Composition id="Scene06OmniRoute" component={Scene06OmniRoute} durationInFrames={240} {...sceneDefaults} />
        <Composition id="Scene07Choice" component={Scene07Choice} durationInFrames={270} {...sceneDefaults} />
        <Composition id="Scene08Final" component={Scene08Final} durationInFrames={210} {...sceneDefaults} />
      </Folder>
    </>
  );
};
