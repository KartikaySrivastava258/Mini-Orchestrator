import { Link, useLocation } from "react-router-dom";
import { EmptyState } from "@/components/console/States";

export default function NotFound() {
  const { pathname } = useLocation();
  return (
    <EmptyState title="Page not found" action={<Link className="text-primary underline" to="/">Go to the overview</Link>}>
      There is no console page at <span className="ident">{pathname}</span>.
    </EmptyState>
  );
}
