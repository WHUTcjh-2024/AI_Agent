// Generated from config/schools; run npm run timetable:bundle. Public adapter configuration only.
import type { SchoolAdapter } from './school-adapter';
export const schoolAdapter: SchoolAdapter = {
  "schoolId": "whut",
  "schoolName": "武汉理工大学",
  "timetable": {
    "enabled": true,
    "provider_id": "whut-bachelor",
    "label": "武汉理工大学教务系统",
    "timezone": "Asia/Shanghai",
    "login_url": "https://zhlgd.whut.edu.cn/tpass/login?service=https%3A%2F%2Fjwxt.whut.edu.cn%2Fjwapp%2Fsys%2Fhomeapp%2Findex.do%3FforceCas%3D1",
    "allowed_hosts": [
      "zhlgd.whut.edu.cn",
      "jwxt.whut.edu.cn"
    ],
    "origin": "https://jwxt.whut.edu.cn",
    "import_path": "/jwapp/sys/homeapp/",
    "role_path": "/jwapp/sys/homeapp/api/home/changeAppRole.do?appRole=ef212c48c8f84be79acbd9d81b090f51",
    "user_path": "/jwapp/sys/homeapp/api/home/currentUser.do",
    "courses_path": "/jwapp/sys/kcbcxby/modules/xskcb/cxxskcb.do",
    "calendar_path": "/jwapp/sys/kcbcxby/modules/xskcb/cxxljc.do"
  }
};
