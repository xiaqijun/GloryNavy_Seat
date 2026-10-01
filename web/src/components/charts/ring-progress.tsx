import { useEffect, useRef } from "react";
import { init, use as registerCharts } from "echarts/core";
import { PieChart } from "echarts/charts";
import { SVGRenderer } from "echarts/renderers";

import { getLocale } from "@/lib/i18n";
import "./ring-progress.css";

registerCharts([PieChart, SVGRenderer]);

export default function RingProgress({
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
      const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches;
      const styles = getComputedStyle(element);
      const primary = styles.getPropertyValue("--primary").trim() || "#2563eb";
      const track = styles.getPropertyValue("--border").trim() || "#e2e8f0";
      const ratio = target > 0 ? Math.max(0, Math.min(value / target, 1)) : 0;
      chart.resize();
      chart.setOption(
        {
          animation: !reduced,
          animationDuration: 180,
          series: [
            {
              type: "pie",
              radius: ["70%", "86%"],
              center: ["50%", "50%"],
              silent: true,
              clockwise: true,
              startAngle: 90,
              label: { show: false },
              itemStyle: { borderWidth: 0 },
              data: [
                { value: ratio, itemStyle: { color: primary } },
                { value: 1 - ratio, itemStyle: { color: track } },
              ],
            },
          ],
        },
        true,
      );
    };
    draw();
    const observer = new ResizeObserver(draw);
    observer.observe(element);
    return () => {
      observer.disconnect();
      chart.dispose();
    };
  }, [target, value]);
  return (
    <div
      className="ring-progress-shell"
      role="img"
      aria-label={`${label}: ${value} / ${target} PAP`}
    >
      <div ref={ref} className="ring-progress" aria-hidden="true" />
      <strong className="ring-progress-value">
        {value.toLocaleString(getLocale())}
      </strong>
    </div>
  );
}
