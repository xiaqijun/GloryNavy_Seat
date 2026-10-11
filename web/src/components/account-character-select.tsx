import { msg } from "@/lib/i18n";

export type AccountCharacterOption = {
  id: string;
  name: string;
  account_id?: string;
  nickname?: string;
  is_main?: boolean;
  corporation_id?: string;
};

function groupKey(character: AccountCharacterOption) {
  return character.account_id || character.nickname || "__account__";
}

function groupLabel(character: AccountCharacterOption) {
  return character.nickname?.trim() || msg("当前账号");
}

/** Select an account first only when the list spans multiple accounts. */
export function AccountCharacterSelect({
  label,
  characters,
  value,
  onChange,
  autoFocus = false,
  disabled = false,
}: {
  label: string;
  characters: AccountCharacterOption[];
  value: string;
  onChange: (value: string) => void;
  autoFocus?: boolean;
  disabled?: boolean;
}) {
  const selected = characters.find((character) => character.id === value);
  const groups = Array.from(
    new Map(
      characters.map((character) => [groupKey(character), character]),
    ).entries(),
  );
  const selectedKey = selected ? groupKey(selected) : groups[0]?.[0] || "";
  const visible = characters.filter((character) => groupKey(character) === selectedKey);
  const first = visible[0]?.id || "";
  const showAccount = groups.length > 1;
  const roleSelect = (
    <select
      autoFocus={autoFocus}
      value={selected && selectedKey === groupKey(selected) ? value : first}
      onChange={(event) => onChange(event.target.value)}
      required
      disabled={disabled}
      aria-label={showAccount ? `${label} · ${msg("角色")}` : label}
    >
      <option value="" disabled>{msg("请选择角色")}</option>
      {visible.map((character) => (
        <option key={character.id} value={character.id}>
          {character.name}{character.is_main ? `（${msg("主角色")}）` : ""}
        </option>
      ))}
    </select>
  );
  if (!showAccount) {
    return (
      <div className="account-character-select account-character-select-single">
        <label>
          {label}
          {roleSelect}
        </label>
      </div>
    );
  }
  return (
    <div className="account-character-select" role="group" aria-label={label}>
      <span className="account-character-select-title">{label}</span>
      <div className="account-character-select-fields">
      <label>
        {msg("昵称")}
        <select
          value={selectedKey}
          onChange={(event) => {
            const next = characters.find(
              (character) => groupKey(character) === event.target.value,
            );
            if (next) onChange(next.id);
          }}
          required
          disabled={disabled}
          aria-label={`${label} · ${msg("昵称")}`}
        >
          <option value="" disabled>{msg("请选择昵称")}</option>
          {groups.map(([key, character]) => (
            <option key={key} value={key}>{groupLabel(character)}</option>
          ))}
        </select>
      </label>
      <label>
        {msg("角色")}
        {roleSelect}
      </label>
      </div>
    </div>
  );
}
