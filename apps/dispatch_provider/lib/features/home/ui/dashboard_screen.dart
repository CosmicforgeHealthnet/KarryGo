import 'package:flutter/material.dart';
import 'home_screen.dart';
import '../../requests/ui/requests_screen.dart';
import '../../bookings/ui/bookings_screen.dart';
import '../../wallet/ui/wallet_screen.dart';
import '../../profile/ui/profile_screen.dart';

class DashboardScreen extends StatefulWidget {
  const DashboardScreen({super.key});

  @override
  State<DashboardScreen> createState() => _DashboardScreenState();
}

class _DashboardScreenState extends State<DashboardScreen> {
  int _index = 0;

  final _screens = const [
    HomeScreen(),
    RequestsScreen(),
    BookingsScreen(),
    WalletScreen(),
    ProfileScreen(),
  ];

  static const _icons = [
    Icons.home_rounded,
    Icons.swap_vert,
    Icons.calendar_month_outlined,
    Icons.credit_card_outlined,
    Icons.person_outline,
  ];

  static const _labels = [
    'Home',
    'Requests',
    'Bookings',
    'Wallet',
    'Profile',
  ];

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      extendBody: true,
      body: _screens[_index],
      bottomNavigationBar: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(16, 0, 16, 12),
          child: Container(
            height: 70,
            decoration: BoxDecoration(
              color: Colors.white,
              borderRadius: BorderRadius.circular(999),
              boxShadow: [
                BoxShadow(
                  color: Colors.black.withValues(alpha: 0.10),
                  blurRadius: 20,
                  offset: const Offset(0, 4),
                ),
              ],
            ),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.spaceAround,
              children: List.generate(5, (i) {
                final isActive = _index == i;
                return GestureDetector(
                  onTap: () => setState(() => _index = i),
                  behavior: HitTestBehavior.opaque,
                  child: AnimatedContainer(
                    duration: const Duration(milliseconds: 200),
                    padding: isActive
                        ? const EdgeInsets.symmetric(horizontal: 20, vertical: 12)
                        : const EdgeInsets.all(12),
                    decoration: BoxDecoration(
                      color: isActive
                          ? const Color(0xFF4CAF50)
                          : Colors.transparent,
                      borderRadius: BorderRadius.circular(999),
                    ),
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Icon(
                          _icons[i],
                          size: 22,
                          color: isActive
                              ? Colors.white
                              : const Color(0xFF1A1A1A),
                        ),
                        if (isActive) ...[
                          const SizedBox(width: 8),
                          Text(
                            _labels[i],
                            style: const TextStyle(
                              fontSize: 14,
                              fontWeight: FontWeight.w700,
                              color: Colors.white,
                            ),
                          ),
                        ],
                      ],
                    ),
                  ),
                );
              }),
            ),
          ),
        ),
      ),
    );
  }
}