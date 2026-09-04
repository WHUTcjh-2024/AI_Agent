// Public build-time configuration; school policy and KB secrets stay server-side.
export interface SchoolAdapter {
  schoolId: string;
  schoolName: string;
  timetable: {
    enabled: boolean;
    provider_id: string;
    label: string;
    timezone: string;
    login_url: string;
    allowed_hosts: string[];
    origin: string;
    import_path: string;
    role_path: string;
    user_path: string;
    courses_path: string;
    calendar_path: string;
  };
}
