import AdminShell from '../components/admin/AdminShell'
import { AdminOverviewPage } from '../components/admin/AdminSections'

export default function AdminPage() {
  return (
    <AdminShell section="overview">
      {({ onUnauthorized }) => <AdminOverviewPage onUnauthorized={onUnauthorized} />}
    </AdminShell>
  )
}
