import AdminShell from '../../components/admin/AdminShell'
import { AdminDataPage } from '../../components/admin/AdminSections'

export default function AdminDataRoute() {
  return (
    <AdminShell section="data">
      {({ onUnauthorized }) => <AdminDataPage onUnauthorized={onUnauthorized} />}
    </AdminShell>
  )
}
