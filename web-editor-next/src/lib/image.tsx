import type { CSSProperties, ImgHTMLAttributes } from "react";

interface ImageProps extends ImgHTMLAttributes<HTMLImageElement> {
  fill?: boolean;
  priority?: boolean;
  unoptimized?: boolean;
}

export default function Image({ fill, priority: _, unoptimized: __, style, ...props }: ImageProps) {
  const fillStyle: CSSProperties | undefined = fill
    ? { position: "absolute", inset: 0, width: "100%", height: "100%", objectFit: "cover" }
    : undefined;

  return <img {...props} style={fillStyle ? { ...fillStyle, ...style } : style} />;
}
