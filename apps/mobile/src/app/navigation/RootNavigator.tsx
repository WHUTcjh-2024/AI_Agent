import { Ionicons } from '@expo/vector-icons';
import { createBottomTabNavigator } from '@react-navigation/bottom-tabs';
import { NavigationContainer } from '@react-navigation/native';
import { createNativeStackNavigator } from '@react-navigation/native-stack';
import type { ComponentProps } from 'react';

import { ChatScreen } from '../../screens/Chat/ChatScreen';
import { HistoryScreen } from '../../screens/History/HistoryScreen';
import { HomeScreen } from '../../screens/Home/HomeScreen';
import { ProfileScreen } from '../../screens/Profile/ProfileScreen';
import { SourceDetailScreen } from '../../screens/Source/SourceDetailScreen';
import { colors, typography } from '../../theme';
import type { MainTabParamList, RootStackParamList } from '../../types/navigation';

const Stack = createNativeStackNavigator<RootStackParamList>();
const Tabs = createBottomTabNavigator<MainTabParamList>();
type IconName = ComponentProps<typeof Ionicons>['name'];

const tabIcons: Record<keyof MainTabParamList, { active: IconName; inactive: IconName }> = {
  Home: { active: 'home', inactive: 'home-outline' },
  History: { active: 'time', inactive: 'time-outline' },
  Profile: { active: 'person', inactive: 'person-outline' },
};

function MainTabs() {
  return (
    <Tabs.Navigator
      screenOptions={({ route }) => ({
        headerShown: false,
        tabBarActiveTintColor: colors.accent,
        tabBarInactiveTintColor: colors.textMuted,
        tabBarHideOnKeyboard: true,
        tabBarLabelStyle: { ...typography.metadata, fontWeight: '600' },
        tabBarStyle: { borderTopColor: colors.border, backgroundColor: colors.surface },
        tabBarIcon: ({ color, focused, size }) => (
          <Ionicons name={focused ? tabIcons[route.name].active : tabIcons[route.name].inactive} color={color} size={size} />
        ),
      })}
    >
      <Tabs.Screen component={HomeScreen} name="Home" options={{ tabBarLabel: '首页' }} />
      <Tabs.Screen component={HistoryScreen} name="History" options={{ tabBarLabel: '历史' }} />
      <Tabs.Screen component={ProfileScreen} name="Profile" options={{ tabBarLabel: '我的' }} />
    </Tabs.Navigator>
  );
}

export function RootNavigator() {
  return (
    <NavigationContainer>
      <Stack.Navigator
        screenOptions={{
          animation: 'slide_from_right',
          contentStyle: { backgroundColor: colors.background },
          headerBackButtonDisplayMode: 'minimal',
          headerShadowVisible: false,
          headerStyle: { backgroundColor: colors.background },
          headerTintColor: colors.textPrimary,
          headerTitleStyle: { fontSize: 17, fontWeight: '700' },
        }}
      >
        <Stack.Screen component={MainTabs} name="MainTabs" options={{ headerShown: false }} />
        <Stack.Screen component={ChatScreen} name="Chat" options={{ title: 'AskU' }} />
        <Stack.Screen component={SourceDetailScreen} name="SourceDetail" options={{ title: '来源详情' }} />
      </Stack.Navigator>
    </NavigationContainer>
  );
}
