import { getLocale } from "@/lib/i18n";

export const securityMoney = (minor: number) => `${new Intl.NumberFormat(getLocale(), { maximumFractionDigits: 2 }).format(minor / 100)} ISK`;
