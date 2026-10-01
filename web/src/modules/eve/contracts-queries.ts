import { queryOptions } from "@tanstack/react-query";
import { getContract, getContracts, type Owner } from "./contracts-api";

export function contractListQuery(
  user: string,
  owner: Owner,
  params: URLSearchParams,
) {
  const filter = new URLSearchParams();
  for (const key of ["q", "type", "status", "before"])
    filter.set(key, params.get(key) ?? "");
  return queryOptions({
    queryKey: [
      "eve",
      "contracts",
      "list",
      user,
      owner.kind,
      owner.id,
      filter.toString(),
    ],
    queryFn: ({ signal }) => getContracts(owner, filter, signal),
  });
}

export function contractDetailQuery(user: string, owner: Owner, id: string) {
  return queryOptions({
    queryKey: ["eve", "contracts", "detail", user, owner.kind, owner.id, id],
    queryFn: ({ signal }) => getContract(owner, id, signal),
  });
}
