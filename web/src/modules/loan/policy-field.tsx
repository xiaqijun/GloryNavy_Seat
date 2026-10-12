import { cloneElement, useId, useState, type ReactElement } from "react";
import { CircleHelp } from "lucide-react";
import { Popover, Tooltip } from "radix-ui";
import { msg } from "@/lib/i18n";

export function PolicyField({ label, help, children }: {
  label: string;
  help: string;
  children: ReactElement<{ id?: string }>;
}) {
  const id = useId();
  const [expanded, setExpanded] = useState(false);
  const [hovered, setHovered] = useState(false);
  return <div className="loan-policy-field">
    <div className="loan-policy-label">
      <label htmlFor={id}>{label}</label>
      <Popover.Root open={expanded} onOpenChange={setExpanded}>
        <Tooltip.Provider delayDuration={250}>
          <Tooltip.Root open={!expanded && hovered} onOpenChange={setHovered}>
            <Tooltip.Trigger asChild>
              <Popover.Trigger asChild>
                <button type="button" className="loan-policy-help" aria-label={msg("查看{0}说明", label)}>
                  <CircleHelp size={15} aria-hidden="true" />
                </button>
              </Popover.Trigger>
            </Tooltip.Trigger>
            <Tooltip.Portal>
              <Tooltip.Content className="icon-tooltip loan-policy-tooltip" sideOffset={8} collisionPadding={16}>{help}</Tooltip.Content>
            </Tooltip.Portal>
          </Tooltip.Root>
        </Tooltip.Provider>
        <Popover.Portal>
          <Popover.Content className="icon-tooltip loan-policy-tooltip" sideOffset={8} collisionPadding={16}
            aria-label={msg("查看{0}说明", label)} onOpenAutoFocus={(event) => event.preventDefault()}
            onCloseAutoFocus={(event) => { event.preventDefault(); setHovered(false); }}>
            {help}
          </Popover.Content>
        </Popover.Portal>
      </Popover.Root>
    </div>
    {cloneElement(children, { id })}
  </div>;
}
