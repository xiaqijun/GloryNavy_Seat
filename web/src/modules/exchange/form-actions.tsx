import { msg } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
export function FormActions({
  pending,
  close,
  disabled = false,
  label = msg("保存"),
}: {
  pending: boolean;
  close: () => void;
  disabled?: boolean;
  label?: string;
}) {
  return (
    <div className="exchange-toolbar">
      <Button type="submit" disabled={pending || disabled}>
        {pending ? msg("正在提交") : label}
      </Button>
      <Button
        type="button"
        variant="outline"
        disabled={pending}
        onClick={close}
      >
        {msg("取消")}{" "}
      </Button>
    </div>
  );
}
