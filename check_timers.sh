#!/bin/bash
# Script to check if timer units have the incorrect Requires dependency

echo "Checking timer units for incorrect Requires dependency..."
echo ""

has_issues=false

# Check all mirrorvault timer files (excluding cleanup timer)
for timer in /etc/systemd/system/mirrorvault-*.timer; do
    if [ -f "$timer" ]; then
        # Skip cleanup timer - it correctly requires its service
        if [[ "$timer" == *"mirrorvault-cleanup.timer" ]]; then
            continue
        fi
        
        timer_name=$(basename "$timer")
        
        # Check if it has the incorrect Requires line
        if grep -q "Requires=mirrorvault-cleanup.service" "$timer"; then
            echo "❌ $timer_name - HAS ISSUE (Requires dependency)"
            has_issues=true
        else
            echo "✅ $timer_name - OK"
        fi
    fi
done

echo ""
if [ "$has_issues" = true ]; then
    echo "Some timers need to be fixed. Run: sudo ./fix_timers.sh"
else
    echo "All timers look good! They should run at their scheduled times."
fi
