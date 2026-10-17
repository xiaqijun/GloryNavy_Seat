// Older batch IDs are 51 ASCII characters; EVE persists the first 50.
// Keep the stored ID intact and copy exactly the title the matcher accepts.
export function settlementContractTitle(reference: string): string {
  return /^BATCH-\d{8}-[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}$/.test(reference)
    ? reference.slice(0, 50)
    : reference;
}
