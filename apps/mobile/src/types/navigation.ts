import type { NavigatorScreenParams } from '@react-navigation/native';

export type MainTabParamList = {
  Home: undefined;
  History: undefined;
  Profile: undefined;
};

export type RootStackParamList = {
  MainTabs: NavigatorScreenParams<MainTabParamList>;
  Chat: { sessionId?: string; initialQuestion?: string } | undefined;
  SourceDetail: { sourceId: string };
  Timetable: undefined;
};
