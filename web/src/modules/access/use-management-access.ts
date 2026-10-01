import { useQuery } from "@tanstack/react-query";
import { useSession } from "@/modules/identity";
import { getAccountAccess } from "./api";
export function useManagementAccess(enabled = true) {
  const session = useSession(enabled);
  const user = session.data?.session;
  const access = useQuery({
    queryKey: ["access", "me", user?.user_id],
    queryFn: ({ signal }) => getAccountAccess(signal),
    enabled: enabled && !!user,
    refetchInterval: 30_000,
  });
  return {
    session,
    access,
    allowed:
      enabled &&
      access.isSuccess &&
      (access.data.can_manage === true || access.data.administrator),
  };
}
