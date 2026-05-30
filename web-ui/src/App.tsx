import { Route, Routes, NavLink, BrowserRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ToastProvider } from "@/components/ui/toast";
import { Studio } from "./pages/Studio";
import { RunsList } from "./pages/RunsList";
import { RunDetail } from "./pages/RunDetail";
import { Templates } from "./pages/Templates";
import { Dashboard } from "./pages/Dashboard";
import { Compare } from "./pages/Compare";
import { TimelineEditor } from "./pages/TimelineEditor";
import { Projects } from "./pages/Projects";
import { ProjectDetail } from "./pages/ProjectDetail";
import { AudioLibrary } from "./pages/AudioLibrary";
import { Presets } from "./pages/Presets";
import { PresetEditor } from "./pages/PresetEditor";
import { Formats } from "./pages/Formats";
import { FormatEditor } from "./pages/FormatEditor";
import { SUPPORTED_LANGS } from "./i18n";

const qc = new QueryClient();

const Nav = () => {
  const { t } = useTranslation();
  return (
    <nav className="border-b border-border bg-card/50 backdrop-blur sticky top-0 z-10">
      <div className="max-w-6xl mx-auto flex items-center gap-6 px-4 h-12 text-sm">
        <span className="font-semibold">{t("nav.title")}</span>
        <NavLinks />
        <div className="flex-1" />
        <LanguageSwitcher />
      </div>
    </nav>
  );
};

const NavLinks = () => {
  const { t } = useTranslation();
  const item = "px-2 py-1 rounded hover:bg-secondary/40";
  return (
    <>
      <NavLink to="/" end className={item}>{t("nav.studio")}</NavLink>
      <NavLink to="/runs" className={item}>{t("nav.runs")}</NavLink>
      <NavLink to="/projects" className={item}>{t("nav.projects")}</NavLink>
      <NavLink to="/dashboard" className={item}>{t("nav.dashboard")}</NavLink>
      <NavLink to="/presets" className={item}>{t("nav.presets")}</NavLink>
      <NavLink to="/formats" className={item}>{t("nav.formats")}</NavLink>
      <NavLink to="/library/audio" className={item}>{t("nav.audio")}</NavLink>
    </>
  );
};

const LanguageSwitcher = () => {
  const { i18n, t } = useTranslation();
  return (
    <select
      aria-label="UI language"
      value={i18n.resolvedLanguage}
      onChange={(e) => i18n.changeLanguage(e.target.value)}
      className="h-7 rounded border border-border bg-secondary/30 px-2 text-xs"
    >
      {SUPPORTED_LANGS.map((lng) => (
        <option key={lng} value={lng}>
          {t(`lang.${lng}`)}
        </option>
      ))}
    </select>
  );
};

export default function App() {
  return (
    <QueryClientProvider client={qc}>
      <ToastProvider>
      <BrowserRouter>
        <Nav />
        <Routes>
          <Route path="/" element={<Studio />} />
          <Route path="/runs" element={<RunsList />} />
          <Route path="/runs/:id" element={<RunDetail />} />
          <Route path="/runs/:id/timeline" element={<TimelineEditor />} />
          <Route path="/templates" element={<Templates />} />
          <Route path="/dashboard" element={<Dashboard />} />
          <Route path="/compare" element={<Compare />} />
          <Route path="/projects" element={<Projects />} />
          <Route path="/projects/:id" element={<ProjectDetail />} />
          <Route path="/library/audio" element={<AudioLibrary />} />
          <Route path="/presets" element={<Presets />} />
          <Route path="/presets/new" element={<PresetEditor />} />
          <Route path="/presets/:id/edit" element={<PresetEditor />} />
          <Route path="/formats" element={<Formats />} />
          <Route path="/formats/new" element={<FormatEditor />} />
          <Route path="/formats/:id/edit" element={<FormatEditor />} />
        </Routes>
      </BrowserRouter>
      </ToastProvider>
    </QueryClientProvider>
  );
}
