import CalendarMonthOutlined from '@mui/icons-material/CalendarMonthOutlined';
import PlaceOutlined from '@mui/icons-material/PlaceOutlined';
import ArticleOutlined from '@mui/icons-material/ArticleOutlined';
import SummarizeOutlined from '@mui/icons-material/SummarizeOutlined';
import ManageAccountsOutlined from '@mui/icons-material/ManageAccountsOutlined';
export const navigation = [
  { path: '/records', label: '签到记录', icon: CalendarMonthOutlined },
  { path: '/sign', label: '签到', icon: PlaceOutlined },
  { path: '/reports/week', label: '实习周报', icon: ArticleOutlined },
  { path: '/reports/month', label: '实习月报', icon: SummarizeOutlined },
  { path: '/account', label: '账号设置', icon: ManageAccountsOutlined },
];
