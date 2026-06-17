import AdminShell from '../../components/admin/AdminShell'
import { AdminOpsPage } from '../../components/admin/AdminSections'

export default function AdminOpsRoute() {
  return (
    <AdminShell section="ops">
      {({ onUnauthorized }) => <AdminOpsPage onUnauthorized={onUnauthorized} />}
    </AdminShell>
  )
}
