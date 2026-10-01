import { msg } from "@/lib/i18n";
import type {
  BarSeriesOption,
  XAXisComponentOption,
  YAXisComponentOption,
} from "echarts";

export interface ComparisonDatum {
  name: string;
  value: number | null;
}

export function progressOption(
  element: HTMLElement,
  value: number,
  target: number,
) {
  const base = comparisonOption(element, []);
  return {
    ...base,
    grid: { left: 0, right: 0, top: 0, bottom: 0 },
    xAxis: { type: "value" as const, min: 0, max: target, show: false },
    yAxis: { type: "category" as const, data: [""], show: false },
    series: [
      {
        type: "bar" as const,
        data: [Math.max(0, Math.min(value, target))],
        barWidth: 12,
        showBackground: true,
        backgroundStyle: {
          color: base.xAxis.splitLine.lineStyle.color,
          borderRadius: 6,
        },
        itemStyle: { color: base.series[0].itemStyle.color, borderRadius: 6 },
        silent: true,
        emphasis: { disabled: true },
      },
    ],
  };
}

export function comparisonOption(
  element: HTMLElement,
  data: ComparisonDatum[],
) {
  const style = getComputedStyle(element);
  const primary = style.getPropertyValue("--primary").trim();
  const muted = style.getPropertyValue("--muted").trim();
  const border = style.getPropertyValue("--border").trim();
  const foreground = style.getPropertyValue("--foreground").trim();
  const fontFamily = style.fontFamily;
  const small = element.clientWidth < 420;
  return {
    animation: !matchMedia("(prefers-reduced-motion: reduce)").matches,
    animationDuration: 200,
    animationDurationUpdate: 200,
    animationEasing: "cubicOut" as const,
    animationEasingUpdate: "cubicOut" as const,
    textStyle: { fontFamily, color: foreground },
    backgroundColor: "transparent",
    grid: { left: small ? 94 : 136, right: 42, top: 12, bottom: 28 },
    xAxis: {
      type: "value",
      min: 0,
      minInterval: 1,
      axisLabel: { color: muted, fontSize: 12, hideOverlap: true },
      splitLine: { lineStyle: { color: border, type: "dashed" } },
    } satisfies XAXisComponentOption,
    yAxis: {
      type: "category",
      inverse: true,
      data: data.map((d) => d.name),
      axisTick: { show: false },
      axisLine: { show: false },
      axisLabel: {
        color: foreground,
        fontSize: 12,
        width: small ? 86 : 126,
        overflow: "truncate",
      },
    } satisfies YAXisComponentOption,
    series: [
      {
        type: "bar",
        data: data.map((d) => d.value),
        barMaxWidth: 18,
        itemStyle: { color: primary, borderRadius: [0, 4, 4, 0] },
        label: {
          show: true,
          position: "right",
          color: foreground,
          fontFamily,
          fontSize: 12,
        },
        emphasis: { disabled: true },
      } satisfies BarSeriesOption,
    ],
  };
}

// Daily columns share the exact comparison palette/type/motion contract.
export function dailyOption(element: HTMLElement, data: ComparisonDatum[]) {
  const base = comparisonOption(element, data);
  return {
    ...base,
    grid: { left: 42, right: 12, top: 24, bottom: 28 },
    xAxis: {
      ...base.yAxis,
      inverse: false,
      data: data.map((d) => d.name),
      axisLabel: {
        ...base.xAxis.axisLabel,
        interval: data.length > 7 ? Math.ceil(data.length / 6) - 1 : "auto",
      },
    } satisfies XAXisComponentOption,
    yAxis: {
      ...base.xAxis,
      name: msg("小时"),
      nameTextStyle: { color: base.xAxis.axisLabel.color, fontSize: 12 },
      minInterval: 0,
    } satisfies YAXisComponentOption,
    series: [
      {
        ...base.series[0],
        barMaxWidth: 28,
        itemStyle: { ...base.series[0].itemStyle, borderRadius: [4, 4, 0, 0] },
        label: {
          ...base.series[0].label,
          show: data.length <= 7 && element.clientWidth > 420,
          position: "top",
        },
      } satisfies BarSeriesOption,
    ],
  };
}
