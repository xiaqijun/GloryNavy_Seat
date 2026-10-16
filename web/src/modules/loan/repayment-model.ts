export type RepaymentChoice = {
  mode: "term" | "date" | "installments";
  term: string;
  customDays: string;
  date: string;
  count: string;
  interval: string;
};

export const initialRepayment: RepaymentChoice = { mode: "term", term: "30", customDays: "90", date: "", count: "2", interval: "30" };

export function repaymentInput(value: RepaymentChoice) {
  if (value.mode === "date") return { installment_count: 1, interval_days: 0, repayment_date: value.date };
  if (value.mode === "installments") return { installment_count: Number(value.count), interval_days: Number(value.interval) };
  return { installment_count: 1, interval_days: Number(value.term === "custom" ? value.customDays : value.term) };
}

export function repaymentDateBound(offset: number, now = new Date()) {
  const date = new Date(now.getTime() + (8 + offset * 24) * 60 * 60 * 1000);
  return date.toISOString().slice(0, 10);
}

