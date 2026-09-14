import 'dart:async';

import 'package:flutter/material.dart';

import 'models/echosync_models.dart';
import 'services/echosync_client.dart';

class IcebreakerScreen extends StatefulWidget {
  final EchoMatch? match;
  final EchoSyncClient? client;

  const IcebreakerScreen({super.key, this.match, this.client});

  @override
  State<IcebreakerScreen> createState() => _IcebreakerScreenState();
}

class _IcebreakerScreenState extends State<IcebreakerScreen> {
  late EchoMatch _match;
  int _remaining = 900;
  String _icebreaker = '';
  bool _loadingIcebreaker = false;
  Timer? _ticker;

  @override
  void initState() {
    super.initState();
    _match = widget.match ?? _placeholderMatch();
    _icebreaker = _match.icebreaker;
    _remaining = _match.remainingSeconds.clampInt(0);

    _ticker = Timer.periodic(const Duration(seconds: 1), (_) {
      if (!mounted) return;
      setState(() {
        _remaining = _match.remainingSeconds.clampInt(0);
      });
    });
  }

  static EchoMatch _placeholderMatch() => EchoMatch(
        id: 'demo_a:usr_7x2',
        peerUserId: 'USR_7x2',
        sharedTokens: const ['RUST PROGRAMMING', 'FORMULA 1'],
        similarity: 0.943,
        distanceM: 6.0,
        icebreaker:
            '"Ask them what their favorite Rust crate is for async networking and why they prefer it over Go."',
        createdAt: DateTime.now(),
        expiresAt: DateTime.now().add(const Duration(minutes: 15)),
        ttlSeconds: 900,
      );

  Future<void> _regenerateIcebreaker() async {
    setState(() => _loadingIcebreaker = true);
    try {
      final client = widget.client ?? EchoSyncClient(userId: _match.peerUserId);
      final text = await client.fetchIcebreaker(
        matchId: _match.id,
        forUser: _match.peerUserId,
      );
      if (text.isNotEmpty && mounted) setState(() => _icebreaker = text);
    } catch (_) {
      // network/edge offline: keep the last known icebreaker
    } finally {
      if (mounted) setState(() => _loadingIcebreaker = false);
    }
  }

  @override
  void dispose() {
    _ticker?.cancel();
    super.dispose();
  }

  String get _countdownLabel {
    final s = _remaining;
    final m = s ~/ 60;
    final sec = s % 60;
    return '${m.toString().padLeft(2, '0')}:${sec.toString().padLeft(2, '0')}';
  }

  @override
  Widget build(BuildContext context) {
    final shared = _match.sharedTokens.isNotEmpty
        ? _match.sharedTokens.first
        : 'shared interest';

    return Container(
      decoration: const BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            Color(0xFF121A29),
            Color(0xFF17243A),
            Color(0xFF17243A),
          ],
        ),
      ),

      child: SafeArea(
        child: Column(
          children: [
            // Pink line
            Container(
              height: 2,
              width: double.infinity,
              color: const Color(0xFFE7839A),
            ),

            // Header
            Container(
              height: 62,
              padding: const EdgeInsets.symmetric(horizontal: 24),
              decoration: const BoxDecoration(
                border: Border(
                  bottom: BorderSide(color: Color(0xFF657187), width: 1),
                ),
              ),

              child: Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  const Text(
                    'EchoSync',
                    style: TextStyle(
                      fontSize: 19,
                      fontWeight: FontWeight.bold,
                    ),
                  ),
                  Text(
                    _countdownLabel,
                    style: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                ],
              ),
            ),

            // Content
            Expanded(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(28, 36, 28, 20),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Text(
                      'shared interest',
                      style: TextStyle(fontSize: 13, color: Color(0xFFB5C0D0)),
                    ),

                    const SizedBox(height: 14),

                    Text(
                      shared,
                      style: const TextStyle(
                        fontSize: 31,
                        fontWeight: FontWeight.w400,
                        letterSpacing: -1,
                      ),
                    ),

                    const SizedBox(height: 14),

                    Row(
                      children: [
                        Expanded(
                          child: Container(height: 1, color: const Color(0xFFB3BDCD)),
                        ),
                        const SizedBox(width: 14),
                        Text(
                          'similarity ${_match.similarity.toStringAsFixed(3)}',
                          style: const TextStyle(
                            fontSize: 12,
                            color: Color(0xFFD0D7E1),
                          ),
                        ),
                      ],
                    ),

                    const SizedBox(height: 48),

                    Row(
                      children: const [
                        Text(
                          'icebreaker · on-device',
                          style: TextStyle(fontSize: 13, color: Color(0xFFB5C0D0)),
                        ),
                      ],
                    ),

                    const SizedBox(height: 28),

                    GestureDetector(
                      onTap: _loadingIcebreaker ? null : _regenerateIcebreaker,
                      child: Text(
                        _loadingIcebreaker ? 'generating…' : _icebreaker,
                        style: const TextStyle(
                          fontSize: 18,
                          height: 1.55,
                          fontWeight: FontWeight.w600,
                          color: Color(0xFFE8EBEF),
                        ),
                      ),
                    ),

                    const Spacer(),

                    Container(height: 1, color: const Color(0xFF788598)),

                    const SizedBox(height: 16),

                    SizedBox(
                      width: double.infinity,
                      height: 52,
                      child: OutlinedButton(
                        onPressed: () {
                          ScaffoldMessenger.of(context).showSnackBar(
                            const SnackBar(content: Text('Hello sent 👋')),
                          );
                        },
                        style: OutlinedButton.styleFrom(
                          side: const BorderSide(color: Color(0xFFD0D7E1)),
                          shape: const RoundedRectangleBorder(
                            borderRadius: BorderRadius.zero,
                          ),
                        ),
                        child: const Text(
                          'say hello',
                          style: TextStyle(
                            fontSize: 14,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                      ),
                    ),

                    const SizedBox(height: 12),

                    Center(
                      child: Text(
                        'connection expires in $_countdownLabel',
                        style: const TextStyle(
                          fontSize: 10,
                          color: Color(0xFF7E899A),
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

extension IntClamp on int {
  int clampInt(int low) => this < low ? low : this;
}