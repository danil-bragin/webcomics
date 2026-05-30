import React from "react";
import { Composition, registerRoot } from "remotion";
import { Comic, ComicProps, totalDurationFrames } from "./compositions/Comic";

const defaultProps: ComicProps = {
  panels: [],
  width: 1080,
  height: 1080,
  fps: 30,
  audioSrc: "",
  musicSrc: "",
};

export const RemotionRoot: React.FC = () => {
  return (
    <>
      <Composition
        id="Comic"
        component={Comic}
        defaultProps={defaultProps}
        width={defaultProps.width}
        height={defaultProps.height}
        fps={defaultProps.fps}
        durationInFrames={300}
        calculateMetadata={({ props }) => {
          const duration = Math.max(1, totalDurationFrames(props.panels, props.fps));
          return {
            durationInFrames: duration,
            width: props.width,
            height: props.height,
            fps: props.fps,
          };
        }}
      />
    </>
  );
};

registerRoot(RemotionRoot);
