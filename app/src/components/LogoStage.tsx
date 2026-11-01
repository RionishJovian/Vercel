/** The brand mark: floats over a soft glow, signal rings leave the Wi-Fi arcs and the wand tip twinkles. Decorative only. */
export function LogoStage({ size = 168 }: { size?: number }) {
  const base = import.meta.env.BASE_URL;
  return (
    <div className="ls" style={{ width: size, height: size }} aria-hidden="true">
      <i className="ls-glow" />
      <i className="ls-ring" /><i className="ls-ring" /><i className="ls-ring" />
      <picture><source srcSet={base + "mark.webp"} type="image/webp" /><img className="ls-mark" src={base + "mark.png"} alt="" width={size} height={size} fetchPriority="high" /></picture>
      <i className="ls-spark s1" /><i className="ls-spark s2" /><i className="ls-spark s3" /><i className="ls-spark s4" />
    </div>
  );
}
