// Test the decimal text without converting ISK to a binary float.
export function hasWithheldTax(value?: string | null): value is string {
  if (!value) return false;
  const match = /^(\d+)(?:\.(\d+))?(?:e([+-]?\d+))?$/i.exec(value);
  return (
    !!match &&
    /[1-9]/.test(match[1] + (match[2] || "")) &&
    Math.abs(Number(match[3] || 0)) <= 100
  );
}
