import { useEffect, useMemo, useRef, useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { api, type RunSummary } from "@/api/client";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { CardSkeletonGrid } from "@/components/ui/skeleton";
import { fmtMoney, statusVariant } from "@/lib/format";

const ALL_STATUSES = ["queued", "running", "completed", "failed", "cancelled"] as const;
const PAGE_SIZE = 18;

export function RunsList() {
  const { t } = useTranslation();
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [search, setSearch] = useState("");
  const statusArr = Array.from(selected);
  const [selectMode, setSelectMode] = useState(false);
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const togglePicked = (id: string) => {
    const next = new Set(picked);
    if (next.has(id)) next.delete(id); else next.add(id);
    setPicked(next);
  };

  const q = useInfiniteQuery({
    queryKey: ["runs", statusArr.sort().join(","), search],
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      api.listRuns({ status: statusArr, q: search, limit: PAGE_SIZE, offset: pageParam as number }),
    getNextPageParam: (lastPage, allPages) => {
      if (!lastPage || lastPage.length < PAGE_SIZE) return undefined;
      return allPages.length * PAGE_SIZE;
    },
    refetchInterval: 4000,
  });

  // Defensive: pages may contain null when the backend returns `null` for an
  // empty list (older deployments) or when a fetch fails mid-flight.
  const items = useMemo<RunSummary[]>(
    () => (q.data?.pages.flat() ?? []).filter((r): r is RunSummary => r != null),
    [q.data],
  );

  // Sentinel for infinite scroll.
  const sentinel = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    if (!sentinel.current) return;
    const obs = new IntersectionObserver((entries) => {
      if (entries[0].isIntersecting && q.hasNextPage && !q.isFetchingNextPage) {
        q.fetchNextPage();
      }
    }, { rootMargin: "300px" });
    obs.observe(sentinel.current);
    return () => obs.disconnect();
  }, [q.hasNextPage, q.isFetchingNextPage, q.fetchNextPage]);

  const toggle = (s: string) => {
    const next = new Set(selected);
    if (next.has(s)) next.delete(s);
    else next.add(s);
    setSelected(next);
  };

  return (
    <div className="max-w-6xl mx-auto p-6 space-y-4">
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between w-full">
            <CardTitle>{t("runs.title")}</CardTitle>
            <div className="flex items-center gap-2 flex-wrap">
              {ALL_STATUSES.map((s) => (
                <Button
                  key={s}
                  variant={selected.has(s) ? "default" : "outline"}
                  className="h-7 px-2 text-xs"
                  onClick={() => toggle(s)}
                >
                  {t(`runs.status.${s}`, s)}
                </Button>
              ))}
              {selected.size > 0 ? (
                <Button variant="ghost" className="h-7 px-2 text-xs" onClick={() => setSelected(new Set())}>
                  {t("runs.clear")}
                </Button>
              ) : null}
              <Button
                variant={selectMode ? "default" : "outline"}
                className="h-7 px-2 text-xs"
                onClick={() => { setSelectMode((v) => !v); if (selectMode) setPicked(new Set()); }}
              >
                {selectMode ? t("runs.exitSelect") : t("runs.select")}
              </Button>
              {selectMode && picked.size >= 1 ? (
                <a
                  href={`/compare?ids=${Array.from(picked).join(",")}`}
                  className="h-7 px-3 text-xs rounded bg-primary text-primary-foreground inline-flex items-center"
                >
                  {t("runs.compareCount", { count: picked.size })}
                </a>
              ) : null}
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <Input
            placeholder={t("runs.searchPrompt")}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="mb-4"
          />
          {q.isLoading ? <CardSkeletonGrid count={6} cols={3} /> : null}
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {items.map((r) => (
              <RunCard
                key={r.id}
                id={r.id}
                prompt={r.prompt}
                status={r.status}
                cost={r.total_cost_usd}
                createdAt={r.created_at}
                videoAssetId={r.video_asset_id}
                selectMode={selectMode}
                picked={picked.has(r.id)}
                onTogglePicked={togglePicked}
              />
            ))}
          </div>
          {items.length === 0 && !q.isLoading ? (
            <p className="text-sm text-muted-foreground mt-3">{t("runs.noMatch")}</p>
          ) : null}
          <div ref={sentinel} className="h-8 flex items-center justify-center mt-4">
            {q.isFetchingNextPage ? (
              <span className="text-xs text-muted-foreground">{t("runs.loadingMore")}</span>
            ) : !q.hasNextPage && items.length > 0 ? (
              <span className="text-xs text-muted-foreground">— {t("runs.end")} —</span>
            ) : null}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}

export function RunCard(props: {
  id: string;
  prompt: string;
  status: string;
  cost: number;
  createdAt: string;
  videoAssetId?: string;
  selectMode?: boolean;
  picked?: boolean;
  onTogglePicked?: (id: string) => void;
}) {
  const { t, i18n } = useTranslation();
  const onClickWrap = (e: React.MouseEvent) => {
    if (props.selectMode) {
      e.preventDefault();
      props.onTogglePicked?.(props.id);
    }
  };
  return (
    <Link
      to={`/runs/${props.id}`}
      onClick={onClickWrap}
      className={`relative rounded-lg border bg-card overflow-hidden flex flex-col transition-colors ${
        props.picked ? "border-primary" : "border-border hover:border-primary/40"
      }`}
    >
      {props.selectMode ? (
        <div className="absolute top-2 left-2 z-10 w-5 h-5 rounded bg-black/60 flex items-center justify-center text-xs">
          {props.picked ? "✓" : ""}
        </div>
      ) : null}
      <VideoTile assetId={props.videoAssetId} status={props.status} />
      <div className="p-3 space-y-1">
        <div className="flex items-center justify-between gap-2">
          <Badge variant={statusVariant(props.status)}>{t(`runs.status.${props.status}`, props.status)}</Badge>
          <span className="text-xs tabular-nums text-muted-foreground">{fmtMoney(props.cost)}</span>
        </div>
        <p className="text-sm truncate" title={props.prompt}>{props.prompt}</p>
        <p className="text-[10px] text-muted-foreground">{new Date(props.createdAt).toLocaleString(i18n.resolvedLanguage)}</p>
      </div>
    </Link>
  );
}

function VideoTile({ assetId, status }: { assetId?: string; status: string }) {
  const { t } = useTranslation();
  const [url, setURL] = useState<string | null>(null);
  const videoRef = useRef<HTMLVideoElement | null>(null);

  useEffect(() => {
    if (!assetId) return;
    let alive = true;
    api.getAssetUrl(assetId).then((r) => alive && setURL(r.url)).catch(() => {});
    return () => { alive = false; };
  }, [assetId]);

  // Seek a hair past 0 to force first-frame paint — many browsers (Safari,
  // Chromium occasionally) show black until decode begins.
  const onLoadedMetadata = () => {
    const v = videoRef.current;
    if (!v) return;
    try {
      v.currentTime = 0.1;
    } catch {}
  };

  if (!assetId) {
    return (
      <div className="aspect-square bg-secondary/20 flex items-center justify-center text-xs text-muted-foreground">
        {status === "running" || status === "queued" ? t("runs.rendering") : t("runs.noVideo")}
      </div>
    );
  }
  if (!url) return <div className="aspect-square bg-secondary/40 animate-pulse" />;

  // Use the poster-only first frame on the list — no hover-play, no darken
  // overlay. Real playback lives on the run detail page.
  return (
    <video
      ref={videoRef}
      src={url}
      className="aspect-square w-full object-cover bg-card"
      muted
      playsInline
      preload="metadata"
      onLoadedMetadata={onLoadedMetadata}
    />
  );
}
