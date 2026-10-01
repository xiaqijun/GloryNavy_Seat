import { msg } from "@/lib/i18n";
import { useEffect, useRef, useState } from "react";
import { init, use as registerCharts } from "echarts/core";
import { BarChart } from "echarts/charts";
import { GridComponent } from "echarts/components";
import { SVGRenderer } from "echarts/renderers";
import { comparisonOption, dailyOption, type ComparisonDatum } from "./theme";

registerCharts([BarChart, GridComponent, SVGRenderer]);

export default function BarComparison({
  data,
  label,
  vertical = false,
}: {
  data: ComparisonDatum[];
  label: string;
  vertical?: boolean;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    const element = ref.current;
    if (!element) return;
    const chart = init(element, undefined, { renderer: "svg" });
    const draw = () => {
      try {
        chart.resize();
        chart.setOption(
          vertical
            ? dailyOption(element, data)
            : comparisonOption(element, data),
          true,
        );
      } catch {
        setFailed(true);
      }
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
  }, [data, vertical]);
  return (
    <>
      <div
        ref={ref}
        style={{
          height: vertical ? 260 : Math.max(140, data.length * 42 + 40),
          minWidth: 0,
        }}
        role="img"
        aria-label={`${label}。${data.map((d) => `${d.name}：${d.value === null ? msg("未计量") : d.value}`).join("；")}`}
      />
      {failed && <p role="status">{msg("图表暂不可用，请查看明细")}</p>}
    </>
  );
}
