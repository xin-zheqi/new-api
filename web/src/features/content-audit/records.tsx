/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogFooter,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useAuthStore } from "@/stores/auth-store";

import { auditRequest, getAuditRecords } from "./api";
import { AuditMatches } from "./matches";
import type { AuditResult } from "./types";

function RecordDetails(props: { id: string }) {
  const { t } = useTranslation();
  const query = useQuery({
    queryKey: ["content-audit-detail", props.id],
    queryFn: () => auditRequest<AuditResult>(`/records/${props.id}`),
    gcTime: 0,
    retry: false,
  });
  if (query.error) return <p role="alert">{query.error.message}</p>;
  if (!query.data) return <p>{t("Loading...")}</p>;
  return <AuditMatches result={query.data} />;
}
export function AuditRecords() {
  const { t } = useTranslation();
  const owner = useAuthStore((state) => state.auth.user?.id);
  const client = useQueryClient();
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState({
    keyword: "",
    action: "",
    start: "",
    end: "",
  });
  const [params, setParams] = useState("");
  const [selected, setSelected] = useState("");
  const [deleting, setDeleting] = useState<string | null>(null);
  const query = useQuery({
    queryKey: ["content-audit-records", owner, params, page],
    gcTime: 0,
    retry: false,
    queryFn: () =>
      getAuditRecords(new URLSearchParams(`${params}&page=${page}`)),
  });
  const remove = useMutation({
    mutationFn: (id: string) =>
      auditRequest(`/records${id ? `/${id}` : ""}`, "DELETE"),
    onSuccess: () => {
      setDeleting(null);
      setSelected("");
      void client.invalidateQueries({ queryKey: ["content-audit-records"] });
    },
  });
  const search = () => {
    const values = new URLSearchParams({
      keyword: filters.keyword,
      action: filters.action,
    });
    if (filters.start) {
      values.set(
        "start_timestamp",
        String(Math.floor(new Date(filters.start).getTime() / 1000)),
      );
    }
    if (filters.end) {
      values.set(
        "end_timestamp",
        String(Math.floor(new Date(filters.end).getTime() / 1000)),
      );
    }
    const next = values.toString();
    if (page === 1 && next === params) void query.refetch();
    setPage(1);
    setParams(next);
  };
  return (
    <div className="flex flex-col gap-4">
      <form
        className="flex flex-wrap items-end gap-2 2xl:flex-nowrap"
        onSubmit={(event) => {
          event.preventDefault();
          search();
        }}
      >
        <FieldGroup className="flex min-w-0 flex-1 flex-wrap gap-2 sm:flex-row 2xl:flex-nowrap">
          <Field className="sm:w-48 sm:shrink-0">
            <FieldLabel htmlFor="audit-search">
              {t("Request ID, key name or model")}
            </FieldLabel>
            <Input
              id="audit-search"
              maxLength={128}
              value={filters.keyword}
              onChange={(event) =>
                setFilters({ ...filters, keyword: event.target.value })
              }
            />
          </Field>
          <Field className="sm:w-28 sm:shrink-0">
            <FieldLabel htmlFor="audit-action">{t("Action")}</FieldLabel>
            <NativeSelect
              id="audit-action"
              value={filters.action}
              onChange={(event) =>
                setFilters({ ...filters, action: event.target.value })
              }
            >
              <NativeSelectOption value="">{t("All")}</NativeSelectOption>
              {["warn", "replace", "block"].map((action) => (
                <NativeSelectOption key={action} value={action}>
                  {t(action)}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </Field>
          <Field className="sm:w-52 sm:shrink-0">
            <FieldLabel htmlFor="audit-start">{t("Start Time")}</FieldLabel>
            <Input
              id="audit-start"
              type="datetime-local"
              value={filters.start}
              onChange={(event) =>
                setFilters({ ...filters, start: event.target.value })
              }
            />
          </Field>
          <Field className="sm:w-52 sm:shrink-0">
            <FieldLabel htmlFor="audit-end">{t("End Time")}</FieldLabel>
            <Input
              id="audit-end"
              type="datetime-local"
              value={filters.end}
              onChange={(event) =>
                setFilters({ ...filters, end: event.target.value })
              }
            />
          </Field>
        </FieldGroup>
        <Button type="submit">{t("Search")}</Button>
        <Button type="button" variant="outline" onClick={() => setDeleting("")}>
          {t("Delete all my records")}
        </Button>
      </form>
      {query.error && (
        <p role="alert" className="text-destructive">
          {query.error.message}
        </p>
      )}
      <Table>
        <TableHeader>
          <TableRow>
            {[
              "Time",
              "API key name",
              "Model",
              "Action",
              "Matches",
              "Request ID",
              "Actions",
            ].map((key) => (
              <TableHead key={key}>{t(key)}</TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {query.data?.items.map((row) => (
            <TableRow key={row.id}>
              <TableCell>
                {new Date(row.created_at * 1000).toLocaleString()}
              </TableCell>
              <TableCell className="max-w-48 truncate">
                {row.token_name}
              </TableCell>
              <TableCell className="max-w-48 truncate">
                {row.model}
                <div className="text-muted-foreground text-xs">
                  {row.endpoint}
                </div>
              </TableCell>
              <TableCell>
                <Badge
                  variant={row.action === "block" ? "destructive" : "secondary"}
                >
                  {t(row.action)}
                </Badge>
              </TableCell>
              <TableCell>{row.match_count}</TableCell>
              <TableCell className="max-w-48 truncate">
                {row.request_id}
              </TableCell>
              <TableCell>
                <Button variant="ghost" onClick={() => setSelected(row.id)}>
                  {t("Details")}
                </Button>
                <Button variant="ghost" onClick={() => setDeleting(row.id)}>
                  {t("Delete")}
                </Button>
              </TableCell>
            </TableRow>
          ))}
          {!query.data?.items.length && (
            <TableRow>
              <TableCell colSpan={7}>
                {query.isLoading ? t("Loading...") : t("No audit records")}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
      <div className="flex items-center justify-end gap-3">
        <span>
          {t("Total")}: {query.data?.total || 0}
        </span>
        <Button
          variant="outline"
          disabled={page <= 1 || query.isFetching}
          onClick={() => setPage(page - 1)}
        >
          {t("Previous")}
        </Button>
        <span>{page}</span>
        <Button
          variant="outline"
          disabled={page * 20 >= (query.data?.total || 0) || query.isFetching}
          onClick={() => setPage(page + 1)}
        >
          {t("Next")}
        </Button>
      </div>
      <Sheet
        open={!!selected}
        onOpenChange={(open) => {
          if (!open) setSelected("");
        }}
      >
        <SheetContent className="w-full sm:max-w-2xl">
          <SheetHeader>
            <SheetTitle>{t("Match details")}</SheetTitle>
          </SheetHeader>
          <div className="min-h-0 flex-1 overflow-auto px-4 pb-4">
            {selected && <RecordDetails key={selected} id={selected} />}
          </div>
        </SheetContent>
      </Sheet>
      <AlertDialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("Delete")}</AlertDialogTitle>
          </AlertDialogHeader>
          {remove.error && <p role="alert">{remove.error.message}</p>}
          <AlertDialogFooter>
            <Button variant="outline" onClick={() => setDeleting(null)}>
              {t("Cancel")}
            </Button>
            <Button
              variant="destructive"
              disabled={remove.isPending}
              onClick={() => remove.mutate(deleting || "")}
            >
              {t("Delete")}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
