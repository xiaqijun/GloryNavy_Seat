import { useEffect, useRef } from "react";
import { init, use as registerCharts } from "echarts/core";
import { LineChart } from "echarts/charts";
import { GridComponent, TooltipComponent } from "echarts/components";
import { SVGRenderer } from "echarts/renderers";
import { getLocale } from "@/lib/i18n";
import { comparisonOption } from "./theme";

registerCharts([LineChart, GridComponent, TooltipComponent, SVGRenderer]);

function chartValue(value: number, unit: string) {
  const normalized = unit.trim();
  const isISK = normalized.endsWith("ISK");
  if (isISK) {
    const scale = normalized.startsWith("M ") ? 1_000_000 : normalized.startsWith("B ") ? 1_000_000_000 : normalized.startsWith("K ") ? 1_000 : 1;
    const isk = value * scale;
    const abs = Math.abs(isk);
    const compactUnit = abs >= 1_000_000_000 ? "B" : abs >= 1_000_000 ? "M" : abs >= 1_000 ? "K" : "ISK";
    const divisor = compactUnit === "B" ? 1_000_000_000 : compactUnit === "M" ? 1_000_000 : compactUnit === "K" ? 1_000 : 1;
    return `${Math.round(isk / divisor).toLocaleString(getLocale())} ${compactUnit === "ISK" ? "ISK" : `${compactUnit} ISK`}`;
  }
  return `${value.toLocaleString(getLocale(), { maximumFractionDigits: 1 })}${normalized ? ` ${normalized}` : ""}`;
}

export default function TrendChart({ label, points, unit = "" }: {
  label: string;
  points: { label: string; value: number }[];
  unit?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const element = ref.current;
    if (!element) return;
    const chart = init(element, undefined, { renderer: "svg" });
    const draw = () => {
      const base = comparisonOption(element, []);
      const style = getComputedStyle(element);
      chart.resize();
      chart.setOption({
        animation: base.animation,
        animationDuration: 200,
        animationDurationUpdate: 200,
        textStyle: base.textStyle,
        grid: { left: 8, right: 20, top: 30, bottom: 8, containLabel: true },
        tooltip: {
          trigger: "axis", confine: true,
          backgroundColor: style.getPropertyValue("--card").trim(),
          borderColor: style.getPropertyValue("--border").trim(),
          textStyle: base.textStyle,
          valueFormatter: (value: unknown) => chartValue(Number(value), unit),
        },
        xAxis: {
          type: "category", boundaryGap: false, data: points.map(point => point.label),
          axisTick: { show: false }, axisLine: { show: false },
          axisLabel: { color: base.xAxis.axisLabel.color, fontSize: 12, margin: 12, hideOverlap: true },
        },
        yAxis: {
          ...base.xAxis, minInterval: 0, splitNumber: 3,
          axisLabel: {
            ...base.xAxis.axisLabel,
            formatter: (value: number) => chartValue(value, unit),
          },
        },
        series: [{
          type: "line", name: label, data: points.map(point => point.value),
          smooth: false, symbol: "circle", symbolSize: 6,
          itemStyle: { color: base.series[0].itemStyle.color },
          lineStyle: { width: 2 }, areaStyle: { opacity: 0.07 },
          emphasis: { scale: false },
        }],
      }, true);
    };
    draw();
    const observer = new ResizeObserver(draw);
    observer.observe(element);
    const motion = matchMedia("(prefers-reduced-motion: reduce)");
    motion.addEventListener("change", draw);
    return () => { observer.disconnect(); motion.removeEventListener("change", draw); chart.dispose(); };
  }, [label, points, unit]);
  return <div ref={ref} style={{ width: "100%", minWidth: 0, height: 200 }} role="img"
    aria-label={`${label}: ${points.map(point => `${point.label}: ${chartValue(point.value, unit)}`).join("; ")}`} />;
}
