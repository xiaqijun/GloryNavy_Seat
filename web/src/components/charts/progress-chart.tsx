import { useEffect, useRef } from "react";
import { init, use as registerCharts } from "echarts/core";
import { BarChart } from "echarts/charts";
import { GridComponent } from "echarts/components";
import { SVGRenderer } from "echarts/renderers";
import { progressOption } from "./theme";

registerCharts([BarChart, GridComponent, SVGRenderer]);

export default function ProgressChart({
  value,
  target,
  label,
}: {
  value: number;
  target: number;
  label: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const element = ref.current;
    if (!element) return;
    const chart = init(element, undefined, { renderer: "svg" });
    const draw = () => {
      chart.resize();
      chart.setOption(progressOption(element, value, target), true);
    };
    draw();
    const observer = new ResizeObserver(draw);
    observer.observe(element);
    const motion = matchMedia("(prefers-reduced-motion: reduce)");
    motion.addEventListener("change", draw);
    return () => {
      observer.disconnect();
      motion.removeEventListener("change", draw);
      chart.dispose();
    };
  }, [value, target]);
  return (
    <div
      ref={ref}
      style={{ height: 20, width: "100%", minWidth: 0 }}
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={target}
      aria-valuenow={Math.max(0, Math.min(value, target))}
      aria-valuetext={`${value} / ${target} PAP`}
    />
  );
}
