import { createRootRoute, createRoute, createRouter } from "@tanstack/react-router";
import { AppShell } from "@/App";
import {
  ProjectOverviewPlaceholder,
  ProjectsHomePlaceholder,
  ProjectRunsPlaceholder,
  ProjectWikiPagePlaceholder,
  ProjectWikiPlaceholder,
} from "@/routes/placeholders";

const rootRoute = createRootRoute({
  component: AppShell,
});

const projectsHomeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: ProjectsHomePlaceholder,
});

const projectOverviewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/projects/$slug",
  component: ProjectOverviewPlaceholder,
});

const projectWikiRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/projects/$slug/wiki",
  component: ProjectWikiPlaceholder,
});

const projectWikiPageRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/projects/$slug/wiki/$",
  component: ProjectWikiPagePlaceholder,
});

const projectRunsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/projects/$slug/runs",
  component: ProjectRunsPlaceholder,
});

const routeTree = rootRoute.addChildren([
  projectsHomeRoute,
  projectOverviewRoute,
  projectWikiRoute,
  projectWikiPageRoute,
  projectRunsRoute,
]);

export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
