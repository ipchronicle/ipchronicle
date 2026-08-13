export const en = {
  translation: {
    appName: "IPChronicle",
    common: {
      cancel: "Cancel",
    },
    navigation: {
      menu: "Primary navigation",
      mobileDescription: "Browse IPChronicle sections.",
      toggleSidebar: "Toggle sidebar",
      systemStatus: "System status",
      systemSettings: "System",
      nodes: "Nodes",
      history: "History",
      notifications: "Notifications",
      account: "Account",
      settings: "Settings",
      networkSettings: "Network probes",
      historySettings: "History and storage",
    },
    language: {
      current: "EN",
      switch: "Switch to Simplified Chinese",
    },
    theme: {
      dark: "Use dark theme",
      light: "Use light theme",
    },
    authentication: {
      section: "Administrator",
      loginTitle: "Sign in",
      login: "Sign in",
      logout: "Sign out",
    },
    settings: {
      section: "Settings",
    },
    account: {
      title: "Account and security",
      profile: "Administrator account",
      profileDetail: "Update the local administrator identity.",
      username: "Username",
      password: "Password",
      currentPassword: "Current password",
      newPassword: "New password",
      save: "Save changes",
      saved: "Account changes saved.",
      language: "Language",
      languageDetail: "Saved for this administrator.",
    },
    totp: {
      title: "Two-factor authentication",
      code: "Six-digit code",
      active: "TOTP is enabled.",
      inactive: "TOTP is disabled.",
      enable: "Enable TOTP",
      confirm: "Confirm TOTP",
      enabled: "TOTP enabled.",
      disable: "Disable TOTP",
      qrCode: "TOTP enrollment QR code",
      secret: "Setup key",
      copySecret: "Copy setup key",
      copied: "Setup key copied.",
    },
    sessions: {
      title: "Administrator sessions",
      detail: "The current session expires after 30 days.",
      revokeAll: "Sign out all sessions",
      confirmTitle: "Sign out every administrator session?",
      confirm: "Sign out all",
    },
    status: {
      section: "Center",
      title: "System status",
      checking: "Checking center status",
      operational: "Operational",
      unavailable: "Unavailable",
      healthyDetail: "The center API and both databases are ready.",
      errorDetail: "The center status could not be loaded.",
      service: "Service",
      version: "Version",
      configSchema: "Configuration schema",
      historySchema: "History schema",
      transport: "Browser transport",
      externalOrigin: "External origin",
      externalOriginMode: {
        automatic: "Automatic",
        custom: "Custom",
      },
      trustedProxy: "Trusted proxy",
      configured: "Configured",
      notConfigured: "Not configured",
      checkedAt: "Checked",
      notAvailable: "Not available",
      retry: "Retry",
      defaultCredentialsTitle: "Default credentials are still active",
      defaultCredentialsDetail:
        "The administrator username and password are both admin.",
      openAccount: "Open account settings",
      httpWarningTitle: "Browser connection uses HTTP",
      httpWarningDetail:
        "Credentials, TOTP codes, and sessions are not protected from network interception.",
    },
    systemSettings: {
      title: "System",
      detail:
        "Manage the external address and review product release metadata.",
      externalOrigin: {
        title: "External address",
        detail:
          "Choose the address used in Agent installation commands. A custom address also enables notification links.",
        loadFailed: "External address settings could not be loaded",
        retry: "Retry",
        automatic: "Use this browser's current address",
        label: "Custom external address",
        valueDetail:
          "Enter an HTTP or HTTPS origin without a path, query, or credentials.",
        required: "Enter a custom external address or enable automatic mode.",
        save: "Save address",
        saving: "Saving address",
        saved: "External address settings saved.",
        mode: {
          automatic: "Automatic",
          custom: "Custom",
        },
      },
      release: {
        title: "Release channel",
        detail:
          "The selected channel controls Center and Agent release discovery together.",
        loadFailed: "Release information could not be loaded",
        retry: "Retry",
        discoveryFailed: "Release discovery is unavailable",
        discoveryErrors: {
          "release-discovery-failed":
            "The official GitHub release or its manifest could not be validated. Existing Agents continue running.",
          "current-version-invalid":
            "This Center build does not contain a valid semantic product version.",
        },
        channelLabel: "Discovery channel",
        channelDetail:
          "Stable is the default. The RC channel also includes official release candidates.",
        saving: "Saving release channel",
        channel: {
          stable: "Stable",
          rc: "Release candidate",
        },
        currentVersion: "Center version",
        currentRevision: "Center source revision",
        checkedAt: "Last discovery check",
        availableVersion: "Discovered version",
        availableRevision: "Discovered source revision",
        publishedAt: "Published",
        noneAvailable: "No compatible release found",
        notAvailable: "Not available",
      },
    },
    nodes: {
      section: "Managed nodes",
      title: "Nodes",
      detail:
        "Enroll Linux Agents and review their current control-plane state.",
      refresh: "Refresh",
      retry: "Retry",
      loadFailed: "Nodes could not be loaded",
      notAvailable: "Not yet reported",
      enrollment: {
        title: "Agent enrollment",
        detail:
          "Control automatic registration and install an Agent with one root command.",
        enabled: "Enrollment enabled",
        disabled: "Enrollment disabled",
        allowRegistration: "Allow automatic registration",
        allowRegistrationDetail:
          "Disabling enrollment does not disconnect Agents that are already registered.",
        command: "Installation command",
        commandDetail:
          "Run as root on a supported Linux node. The command contains the reusable registration key.",
        copy: "Copy command",
        copied: "Installation command copied.",
        rotatedAt: "Key rotated {{value}}",
        rotate: "Rotate key",
        rotateTitle: "Rotate the registration key?",
        rotateDetail:
          "Previously copied installation commands will stop working. Existing Agents are unaffected.",
        rotateConfirm: "Rotate key",
        rotated: "Registration key rotated.",
        generated: "Registration key generated and enrollment enabled.",
        empty: "No registration key exists",
        emptyDetail:
          "Generate one to enable automatic node creation and receive the installation command.",
        generate: "Generate key",
      },
      inventory: {
        title: "Node inventory",
        detail:
          "A node appears after its Agent completes automatic registration.",
        count_one: "{{count}} node",
        count_other: "{{count}} nodes",
        empty: "No nodes are registered",
        emptyDetail:
          "Generate an enrollment key, then run the installation command on a Linux node you manage.",
        node: "Node",
        status: "Status",
        publicAddresses: "Public IPs",
        noPublicAddresses: "Waiting for discovery",
        probeEnabled: "Complete probe enabled",
        probeDisabled: "Complete probe disabled",
        addressUnavailable: "Currently unavailable",
        agent: "Agent",
        sourceRevision: "Source {{value}}",
        configuration: "Configuration",
        lastSeen: "Last seen",
        search: "Search nodes",
        searchPlaceholder:
          "Name, hostname, public IP, version, or source revision",
        noMatches: "No nodes match the current filters",
        clearFilters: "Clear filters",
      },
      quickActions: {
        runProbe: "Run probe",
      },
      updates: {
        filter: "Updates available",
        updateSelected: "Update selected ({{count}})",
        selectAvailable: "Select all updateable visible nodes",
        selectNode: "Select {{name}} for Agent update",
        loadFailed: "Agent update state could not be loaded",
        loadFailedDetail:
          "Node management remains available, but update filtering and update actions are temporarily unavailable.",
        accepted: "Accepted update tasks: {{count}}",
        partial: "Some Agent update tasks were not accepted",
        failed: "Agent update tasks were not accepted",
        available: "Update available: {{version}}",
        target: "Target {{version}}",
        result: "Running {{version}}",
        failureCode: "Failure code: {{code}}",
        updateAction: "Update Agent",
        updateNode: "Update Agent on {{name}}",
        disabledReason: "Enable the node before updating its Agent",
        offlineReason: "The Agent must be online before an update can be sent",
        status: {
          pending: "Waiting for Agent",
          acknowledged: "Received by Agent",
          verifying: "Verifying artifact",
          installing: "Installing",
          restarting: "Restarting Agent",
          succeeded: "Update succeeded",
          failed: "Update failed",
          "rolled-back": "Rolled back",
          rejected: "Update rejected",
          expired: "Delivery expired",
          offlineWithPhase: "Offline · {{phase}}",
        },
        errors: {
          agent_update_node_not_found: "The node no longer exists.",
          agent_update_node_revoked: "The Agent credential is revoked.",
          agent_update_node_disabled: "The node is disabled.",
          agent_update_node_offline: "The Agent is offline.",
          agent_update_task_slot_occupied:
            "Another Center-issued task already occupies this node's task slot.",
          agent_update_unsupported:
            "The Agent does not support this update or product major version.",
          agent_update_not_available:
            "The selected version is not newer than the running Agent.",
          agent_update_target_invalid:
            "The discovered release is no longer a valid update target.",
          unknown: "The update task was rejected for an unknown reason.",
        },
      },
      status: {
        online: "Online",
        offline: "Offline",
        disabled: "Disabled",
        revoked: "Revoked",
      },
      configuration: {
        current: "Current",
        pending: "Pending",
        failed: "Failed",
      },
      sync: {
        start: "Start temporary sync",
        stop: "Stop temporary sync",
        unsupported:
          "The current Agent version does not support temporary sync",
        pending: "Waiting for Agent",
        connected: "Live sync active",
        degraded: "Using normal polling",
        until: "Ends",
      },
      actions: {
        title: "Node actions",
        group: "Node actions for {{name}}",
        network: "Public IPs",
        probe: "Complete probes",
        enable: "Enable node",
        disable: "Pause node",
        revoke: "Revoke Agent credential",
        delete: "Permanently delete node",
        retryDeletion: "Retry permanent deletion",
      },
      revoke: {
        title: "Revoke the Agent credential for {{name}}?",
        detail:
          "This cannot be undone. The Agent stops control polling and local schedules after its next connection; node configuration and history remain available.",
        confirm: "Revoke credential",
      },
      deletion: {
        title: "Permanently delete {{name}}?",
        detail:
          "The node, hidden discovery paths, and node-level state are irreversibly removed, and its Agent identity is revoked. Reports, starred snapshots, and address history assigned to public IPs are retained. The Center does not uninstall the Agent service from the host.",
        confirm: "Permanently delete",
        pending: "Deleting",
        failed: "Deletion failed",
      },
    },
    nodeDetail: {
      section: "Managed node",
      back: "Back to nodes",
      identity: "{{hostname}} · Agent {{version}}",
      notFound: "The node does not exist or has been deleted",
      loadFailed: "The node could not be loaded",
      retry: "Retry",
      tabs: {
        label: "Node sections",
        overview: "Overview",
        network: "Public IPs",
        probe: "Probes",
        changes: "Address changes",
        settings: "Settings",
      },
      overview: {
        loadFailed: "The node overview could not be loaded",
        refresh: "Refresh overview",
        node: {
          title: "Node status",
          detail: "Agent identity and current control-plane state.",
          configuration: "{{status}} · {{applied}}/{{desired}}",
          registered: "Registered",
          capabilities: "Capabilities",
          source: "Source revision",
        },
        sync: {
          title: "Temporary sync",
          detail:
            "Temporarily keep an outbound connection active while adjusting this node.",
          inactive: "Normal polling",
        },
        network: {
          title: "Public IPs",
          detail: "Deduplicated public addresses discovered by this node.",
          empty: "No confirmed public address has been reported.",
          nat: "{{count}} public IP(s) are reached through NAT.",
          open: "Manage public IPs",
        },
        probe: {
          title: "Complete probes",
          detail: "Current schedule and most recent complete-probe state.",
          open: "Open probes",
        },
      },
      changes: {
        refresh: "Refresh address changes",
        loadFailed: "Address changes could not be loaded",
      },
      settings: {
        saved: "Node availability was updated.",
        availability: {
          title: "Node availability",
          detail:
            "Pausing preserves configuration and history while stopping all probe work.",
          toggle: "Enable node",
          enabled: "The node is enabled and may run configured probe work.",
          disabled:
            "The node is paused; configuration and history are retained.",
        },
        agent: {
          title: "Agent",
          detail: "Review the installed Agent and apply an available release.",
          platform: "Platform",
          source: "Source revision",
          capabilities: "Capabilities",
          current: "The installed Agent is current for the selected channel.",
          updateAccepted: "The Agent update task was accepted.",
        },
        danger: {
          title: "Danger zone",
          detail:
            "Credential revocation and permanent deletion have different, irreversible effects.",
          revoked: "The Agent credential was revoked.",
          deletionQueued: "Permanent node deletion was queued.",
        },
      },
    },
    proxySettings: {
      section: "Settings",
      title: "Network probes",
      detail:
        "Manage reusable HTTP, HTTPS, and SOCKS5 proxies. Passwords can be replaced or cleared but are never shown again.",
      refresh: "Refresh",
      retry: "Retry",
      loadFailed: "Network proxy settings could not be loaded",
      empty: "No centrally managed proxies are configured.",
      save: "Save settings",
      discovery: {
        title: "Public address discovery",
        detail:
          "The Agent tries these independent services in order. First observations and suspected changes require agreement from two services.",
        ipv4: "IPv4 services",
        ipv6: "IPv6 services",
        format:
          "Enter one HTTP or HTTPS URL per line. Each list requires 2 to 8 distinct hosts.",
        save: "Save discovery services",
        updated: "Updated {{value}}",
        httpTitle: "A discovery service uses HTTP",
        httpDetail:
          "An unencrypted response can be changed in transit and may create a false address event. Saving remains allowed.",
      },
      create: {
        title: "Add proxy",
        detail:
          "The Center sends credentials only to Agents with a proxy discovery path that references this proxy.",
        submit: "Add proxy",
      },
      fields: {
        name: "Name",
        scheme: "Protocol",
        host: "Host or IP address",
        port: "Port",
        username: "Username",
        password: "Password",
        passwordPlaceholder: "Leave empty to keep the current password",
      },
      password: {
        configured: "Password configured",
        empty: "No password",
        replace: "Replace password",
        clear: "Clear password",
        clearTitle: "Clear the stored proxy password?",
        clearDetail:
          "Agents referencing this proxy will receive a new configuration without the password.",
      },
      delete: {
        action: "Delete proxy",
        title: "Delete this network proxy?",
        detail:
          "A proxy referenced by a proxy discovery path cannot be deleted. Remove those paths first.",
        confirm: "Delete proxy",
      },
    },
    network: {
      section: "Node network",
      title: "Public IPs",
      detail:
        "Review public IPs discovered by this node and choose which addresses receive quality probes.",
      back: "Back to nodes",
      refresh: "Refresh",
      retry: "Retry",
      loadFailed: "Node network state could not be loaded",
      nodeNotFound: "The node does not exist or was deleted",
      family: { ipv4: "IPv4", ipv6: "IPv6" },
      publicAddresses: {
        title: "Discovered public IPs",
        detail:
          "Each public IP appears once; interfaces, source addresses, NAT, and proxy paths are handled automatically.",
        empty: "Waiting for the Agent's first public-address discovery.",
        available: "Available",
        unavailable: "Unavailable",
        firstSeen: "First discovered",
        lastSeen: "Last discovered: {{value}}",
        executionNode: "Current execution node",
        noNode: "No available node",
        nat: "Reached through NAT",
        proxy: "Reached through proxy",
        probeEnabled: "Enable complete probe",
        probeEnabledDetail:
          "Only enabled public IPs are included in complete IPQuality probes.",
        probeOnRediscovery: "Probe after rediscovery",
        probeOnRediscoveryDetail:
          "After this public IP returns from an unavailable state, run one complete probe once the Agent applies its latest configuration.",
        saving: "Saving public IP settings",
      },
      observation: {
        waiting: "Waiting for the first lightweight address observation.",
        unknown: "No confirmed public address",
        proxy: "Proxy path",
        nat: "Likely NAT",
        temporary: "Temporary IPv6 source",
        natDetail:
          "The local source differs from the observed public address. Some upstream DNS or raw mail checks may use the default route or fail to bind.",
        status: { confirmed: "Confirmed", failed: "Check failed" },
        failure: {
          "selector-unavailable":
            "The configured local selector is unavailable.",
          "no-valid-response": "No discovery service returned a valid address.",
          "confirmation-unavailable":
            "A second independent service could not confirm the address.",
          "conflicting-responses":
            "Independent discovery services returned different addresses.",
        },
      },
      addressHistory: {
        title: "Address transitions",
        detail:
          "First observations, confirmed changes, failure boundaries, and recoveries are retained by public IP; gaps that cannot be assigned to an IP remain node-level.",
        empty: "No address transition has been reported.",
        kind: {
          "first-observation": "First observation",
          "address-change": "Address changed",
          "check-failure": "Check failed",
          recovery: "Recovered",
        },
        gap: "History gap",
        nodeLevel: "Node-level",
        gapDetail:
          "{{count}} offline events were discarded (sequence {{first}} to {{last}}).",
      },
      proxyDiscovery: {
        title: "Proxy discovery paths",
        detail:
          "A proxy cannot be inferred from node networking. Choose a stored proxy and address family so the Agent can discover its public IP.",
        noProxies: "No reusable network proxy is configured.",
        openSettings: "Configure network proxies",
        proxy: "Network proxy",
        selectProxy: "Select a proxy",
        family: "Address family",
        add: "Add discovery path",
        empty: "This node has no proxy discovery path.",
        available: "Available",
        unavailable: "Unavailable",
        deletion: { pending: "Deleting", failed: "Deletion failed" },
        delete: {
          action: "Delete",
          title: "Delete this proxy discovery path?",
          detail:
            "The node will stop using ‘{{name}}’ to discover public IPs. Previously discovered public IPs, reports, and address history are retained.",
          confirm: "Delete path",
        },
      },
    },
    probe: {
      section: "Node probes",
      title: "Complete probes",
      detail: "Schedule and inspect full IPQuality runs for this node.",
      back: "Back to nodes",
      refresh: "Refresh",
      retry: "Retry",
      loadFailed: "Complete-probe state could not be loaded",
      nodeNotFound: "The node does not exist or was deleted",
      notAvailable: "Not available",
      runNow: "Run complete probe",
      lowMemory: {
        title: "Complete probes are paused for low memory",
        detail:
          "The Agent reported less than 256 MiB of physical memory. Address checks continue; an administrator can accept the risk in probe settings.",
      },
      unavailable: {
        offline:
          "The node is offline, so an immediate task cannot be delivered.",
        disabled:
          "The node is disabled. Enable it before starting a complete probe.",
        lowMemory:
          "Complete probes are paused until the low-memory override is enabled.",
        running: "A complete-probe run is already active on this node.",
        task: "The node already has an immediate task waiting or running.",
      },
      status: {
        title: "Agent probe state",
        detail: "Latest schedule and execution state reported by the Agent.",
        running: "Running",
        idle: "Idle",
        next: "Next scheduled run",
        last: "Last occurrence",
        trigger: "Trigger",
        memory: "Physical memory",
        lastSkipped: "The last occurrence was skipped: {{reason}}",
        resetApplied:
          "History reset applied {{value}}; {{count}} queued item(s) were discarded.",
      },
      task: {
        title: "Immediate task",
        detail:
          "Delivery and execution state for the latest administrator command.",
        empty: "No immediate complete-probe task has been created.",
        created: "The task is waiting for the Agent.",
        offline: "The task is still active, but the node is currently offline.",
        waiting: "Waiting for Agent",
        createdAt: "Created",
        receivedAt: "Agent received",
        startedAt: "Started",
        completedAt: "Completed",
        openRun: "Open run",
      },
      runs: {
        title: "Recent runs",
        detail:
          "Node-level runs retain their frozen public-IP order and individual outcomes.",
        empty: "No complete probe has been run yet.",
        progress: "{{completed}} of {{total}} complete",
      },
      settings: {
        title: "Probe settings",
        detail:
          "The schedule runs locally on the Agent using a six-field Cron expression.",
        scheduleEnabled: "Enable recurring probes",
        scheduleEnabledDetail:
          "Missed occurrences are skipped and are not run after restart.",
        cron: "Cron expression",
        timezone: "Time zone",
        memoryOverride: "Allow probes below 256 MiB",
        memoryOverrideDetail:
          "Enabling this accepts possible probe failure or node resource exhaustion.",
        save: "Save probe settings",
      },
      trigger: {
        manual: "Manual",
        schedule: "Schedule",
        "address-change": "Address change",
      },
      state: {
        pending: "Waiting",
        acknowledged: "Received",
        running: "Running",
        succeeded: "Succeeded",
        partial: "Partial success",
        failed: "Failed",
        rejected: "Rejected",
        expired: "Expired",
        interrupted: "Interrupted",
        skipped: "Skipped",
      },
      skip: {
        busy: "another probe was active",
        disabled: "probing was disabled",
        "low-memory": "the node was below the memory baseline",
        "no-egress": "no enabled public IP was available",
        missed: "the occurrence was missed",
      },
      failure: {
        download: "Script download",
        selector: "Network path selector",
        adapter: "Proxy adapter",
        process: "Probe process",
        timeout: "Timeout",
        output: "JSON output",
        restart: "Agent restart",
      },
    },
    probeRun: {
      section: "Probe run",
      title: "Complete-probe run",
      detail: "Run details",
      back: "Back to node probes",
      refresh: "Refresh",
      retry: "Retry",
      notFound: "The probe run does not exist or its history was removed",
      loadFailed: "The probe run could not be loaded",
      partial: {
        title: "This run completed with partial success",
        detail:
          "Successful public-IP snapshots remain available alongside failed or skipped sibling executions.",
      },
      summary: {
        title: "Run summary",
        trigger: "Trigger",
        startedAt: "Started",
        completedAt: "Completed",
        progress: "Progress",
      },
      executions: {
        title: "Public-IP executions",
        detail: "Each frozen public IP is attempted at most once in this run.",
        addressUnavailable: "Public IP unavailable",
        sequence: "Public-IP sequence {{value}}",
        startedAt: "Started",
        completedAt: "Completed",
        stage: "Failure stage",
        openSnapshot: "Open report snapshot",
        snapshotPending: "The successful snapshot is still arriving.",
      },
    },
    history: {
      section: "Cross-node index",
      title: "History",
      detail:
        "Browse complete-probe reports and address transitions by node, public IP, and time.",
      retry: "Retry",
      loadFailed: "History could not be loaded",
      addressUnavailable: "Public IP unavailable",
      tabs: {
        label: "History type",
        reports: "Probe reports",
        addresses: "Address changes",
      },
      filters: {
        title: "Filters",
        detail: "Active filters are retained in the current address.",
        clear: "Clear filters",
        node: "Node",
        allNodes: "All nodes",
        egress: "Public IP",
        allEgresses: "All public IPs",
        from: "From",
        to: "To",
        runStatus: "Run result",
        allResults: "All results",
        trigger: "Trigger",
        allTriggers: "All triggers",
        changes: "Field changes",
        allChanges: "All",
        changed: "Changed",
        unchanged: "Unchanged",
        format: "Upstream format",
        allFormats: "All formats",
        eventKind: "Event type",
        allEvents: "All events",
        family: "Address family",
        allFamilies: "All families",
      },
      format: { compatible: "Compatible", mismatch: "Format mismatch" },
      reports: {
        title: "Complete-probe reports",
        count: "{{count}} retained snapshots",
        open: "Open report",
        compare: "Compare snapshots",
        baseline: "First baseline",
        changeCount: "{{count}} changes",
        noChanges: "No field changes",
        formatIssues: "{{count}} format issues",
        current: "Current snapshot",
      },
      addresses: {
        title: "Address changes",
        count: "{{count}} events",
        previous: "Previous: {{value}}",
      },
      columns: {
        owner: "Node and public IP",
        result: "Run result",
        interpretation: "Interpretation",
        time: "Observed",
        actions: "Actions",
        event: "Event",
        address: "Public IP change",
      },
      starred: "Starred",
      gaps: {
        probeTitle: "Probe history gaps",
        addressTitle: "Address history gaps",
        detail:
          "Data dropped from a bounded Agent queue is never presented as continuous history.",
        probeItem: "{{count}} results missing, sequence {{first}} to {{last}}",
        addressItem: "{{count}} events missing, sequence {{first}} to {{last}}",
        nodeLevel: "Node-level",
      },
      formatEvents: {
        title: "Upstream format events",
        detail:
          "Format mismatches, changes, and recoveries found by the fixed field catalog.",
        issueCount: "{{count}} issues",
        kind: {
          mismatch: "Format mismatch detected",
          changed: "Mismatch changed",
          recovered: "Format recovered",
        },
      },
      pagination: {
        previous: "Previous",
        next: "Next",
        page: "Page {{current}} of {{total}}",
      },
      empty: { filtered: "No matching records", none: "No history yet" },
    },
    comparison: {
      back: "Back to history",
      section: "Probe reports",
      title: "Snapshot comparison",
      detail:
        "The earliest and latest snapshots are selected by default. Drag the timeline to choose any retained points.",
      invalid: "Choose a public IP from history to compare its snapshots",
      notFound: "A snapshot does not exist or was removed",
      egressMismatch: "The snapshots belong to different public IPs",
      loadFailed: "The comparison could not be loaded",
      retry: "Retry",
      start: "Start snapshot",
      end: "End snapshot",
      noChanges: "No fields changed between the selected snapshots",
      changeCount_one: "{{count}} change",
      changeCount_other: "{{count}} changes",
      highlightDetail:
        "Changed values are highlighted in their original positions in both full reports.",
      timeline: {
        title: "Time range",
        owner: "{{node}} · {{egress}}",
        range: "Earliest {{first}} · latest {{last}}",
        snapshotCount: "{{count}} snapshots",
        gapCount: "{{count}} history gaps",
        snapshot: "Snapshot",
        starred: "Starred",
        gap: "History gap",
        insufficient:
          "This public IP needs at least two retained snapshots for comparison",
        loadFailed:
          "The snapshot timeline for this public IP could not be loaded",
      },
    },
    snapshot: {
      section: "Probe report",
      title: "Report snapshot",
      back: "Back",
      retry: "Retry",
      notFound: "The snapshot does not exist or was removed",
      loadFailed: "The snapshot could not be loaded",
      star: "Star snapshot",
      unstar: "Unstar snapshot",
      compare: "Compare snapshots",
      pngExport: {
        action: "Export PNG",
        exporting: "Generating PNG",
        failed: "The PNG could not be generated. Try again.",
        copy: "Copy PNG",
        copying: "Copying PNG",
        copied: "PNG copied",
        copyFailed:
          "The PNG could not be copied. Check the browser's clipboard permission.",
      },
      unavailable: "Unavailable",
      actualType: "Actual type {{actual}}; expected {{expected}}",
      summary: {
        title: "Interpretation summary",
        baseline: "First baseline",
        changes: "{{count}} changes",
        noChanges: "No field changes",
        formatIssues: "{{count}} format issues",
        compatible: "Format compatible",
      },
      format: {
        title: "Upstream output format mismatch",
        detail:
          "Incompatible fields are not coerced. Unknown fields remain available in the raw JSON.",
        expected: "Expected type: {{expected}}",
      },
      views: {
        label: "Report view",
        report: "Report",
        raw: "Raw JSON",
        diagnostics: "Format diagnostics {{count}}",
      },
      report: {
        empty: "No usable result is available for this section.",
        overview: {
          unknownAddress: "IP address unavailable",
          detail: "Public-IP sequence {{sequence}} · {{value}}",
          version: "Upstream script version",
          upstreamTime: "Upstream report time",
        },
        basic: {
          title: "Basic information",
          detail: "Address ownership and location returned by MaxMind.",
          asn: "ASN",
          organization: "Organization",
          location: "Location",
          registeredRegion: "Registered region",
          timezone: "Time zone",
          type: "IP type",
        },
        type: {
          title: "IP type attributes",
          detail: "Usage and company types returned by each database.",
          database: "Database",
          usage: "Usage",
          company: "Company",
        },
        scores: {
          title: "Risk scores",
          detail: "Raw scores returned by each upstream data source.",
          disclaimer:
            "IPChronicle does not calculate an aggregate risk score; providers may use different scales.",
          level: {
            veryLow: "Very low",
            low: "Low",
            medium: "Medium",
            elevated: "Elevated",
            suspicious: "Suspicious",
            high: "High",
            veryHigh: "Very high",
            risky: "Risky",
            highRisk: "High risk",
            block: "Block recommended",
          },
        },
        factors: {
          title: "Risk factors",
          detail: "Regions and risk attributes returned by each database.",
          item: "Item",
          yes: "Yes",
          no: "No",
          none: "None",
          names: {
            CountryCode: "Region",
            Proxy: "Proxy",
            Tor: "Tor",
            VPN: "VPN",
            Server: "Server",
            Abuser: "Abuser",
            Robot: "Robot",
          },
        },
        media: {
          title: "Streaming and AI",
          detail:
            "Service availability, detected region, and upstream result type.",
          item: "Item",
          status: "Status",
          region: "Region",
          method: "Method",
          unlocked: "Unlocked",
          blocked: "Blocked",
        },
        mail: {
          title: "Mail and blocklists",
          detail: "Outbound mail connectivity and DNS blocklist results.",
          port25: "Local outbound port 25",
          services: "Mail service connectivity",
          reachable: "Reachable",
          unreachable: "Unreachable",
          dns: "DNS blocklists",
          dnsFields: {
            Total: "Checked",
            Clean: "Clean",
            Marked: "Marked",
            Blacklisted: "Blacklisted",
          },
        },
      },
      fieldStatus: {
        available: "Available",
        unavailable: "No data",
        missing: "Missing",
        incompatible: "Incompatible type",
      },
      issueKind: {
        missing: "Missing field",
        incompatible: "Incompatible type",
        unknown: "Unknown field",
      },
      changes: {
        title: "Changes in this snapshot",
        detail:
          "Only semantic changes with compatible types on both sides are shown.",
      },
      fieldCatalog: {
        unmappedDescription:
          "Known upstream field without a localized description.",
        groups: {
          head: {
            name: "Report metadata",
            description:
              "Identity and generation details from the upstream report.",
          },
          info: {
            name: "IP information",
            description:
              "Network ownership and geographic information for the address.",
          },
          type: {
            name: "Network classification",
            description:
              "Usage and organization classifications from IP databases.",
          },
          score: {
            name: "Risk scores",
            description: "Risk scores reported by upstream data providers.",
          },
          factor: {
            name: "Risk indicators",
            description:
              "Country and risk signals reported by upstream data providers.",
          },
          media: {
            name: "Media services",
            description:
              "Service availability, region, and result classifications.",
          },
          mail: {
            name: "Mail connectivity",
            description: "Outbound mail reachability and DNS blocklist checks.",
          },
        },
        fields: {
          head: {
            ip: {
              name: "IP address",
              description: "Public IP address reported by the upstream probe.",
            },
            command: {
              name: "Probe command",
              description: "Command recorded by the upstream probe.",
            },
            github: {
              name: "Upstream source",
              description: "Source reference recorded by the upstream probe.",
            },
            time: {
              name: "Report time",
              description: "Generation time recorded in the upstream report.",
            },
            version: {
              name: "Probe version",
              description: "Version reported by the upstream probe.",
            },
          },
          info: {
            asn: {
              name: "Autonomous system number",
              description:
                "Autonomous system number associated with the address.",
            },
            organization: {
              name: "Organization",
              description: "Organization associated with the address.",
            },
            latitude: {
              name: "Latitude",
              description: "Reported geographic latitude.",
            },
            longitude: {
              name: "Longitude",
              description: "Reported geographic longitude.",
            },
            dms: {
              name: "DMS coordinates",
              description: "Location in degrees, minutes, and seconds.",
            },
            map: {
              name: "Map reference",
              description: "Upstream map reference for the reported location.",
            },
            timeZone: {
              name: "Time zone",
              description: "Time zone associated with the address.",
            },
            cityName: {
              name: "City",
              description: "Reported city or locality name.",
            },
            cityPostalCode: {
              name: "Postal code",
              description: "Reported postal code for the city or locality.",
            },
            citySubCode: {
              name: "City subdivision code",
              description:
                "Reported subdivision code within the city or locality.",
            },
            citySubdivisions: {
              name: "City subdivisions",
              description: "Reported subdivisions within the city or locality.",
            },
            regionCode: {
              name: "Region code",
              description: "Code of the reported geographic region.",
            },
            regionName: {
              name: "Region",
              description: "Name of the reported geographic region.",
            },
            continentCode: {
              name: "Continent code",
              description: "Code of the reported continent.",
            },
            continentName: {
              name: "Continent",
              description: "Name of the reported continent.",
            },
            registeredRegionCode: {
              name: "Registered-region code",
              description:
                "Code of the region where the address is registered.",
            },
            registeredRegionName: {
              name: "Registered region",
              description:
                "Name of the region where the address is registered.",
            },
            type: {
              name: "Address type",
              description: "Network or address type reported upstream.",
            },
          },
          classification: {
            usage: {
              name: "Usage classification ({{provider}})",
              description:
                "Address usage classification reported by {{provider}}.",
            },
            company: {
              name: "Company classification ({{provider}})",
              description:
                "Organization classification reported by {{provider}}.",
            },
          },
          score: {
            risk: {
              name: "Risk score ({{provider}})",
              description: "Address risk score reported by {{provider}}.",
            },
          },
          factor: {
            countryCode: {
              name: "Country code ({{provider}})",
              description: "Address country code reported by {{provider}}.",
            },
            proxy: {
              name: "Proxy indicator ({{provider}})",
              description:
                "Whether {{provider}} identifies the address as a proxy.",
            },
            tor: {
              name: "Tor indicator ({{provider}})",
              description:
                "Whether {{provider}} identifies the address as a Tor exit.",
            },
            vpn: {
              name: "VPN indicator ({{provider}})",
              description:
                "Whether {{provider}} identifies the address as a VPN.",
            },
            server: {
              name: "Server indicator ({{provider}})",
              description:
                "Whether {{provider}} identifies the address as a server.",
            },
            abuser: {
              name: "Abuse indicator ({{provider}})",
              description:
                "Whether {{provider}} associates the address with abuse.",
            },
            robot: {
              name: "Automation indicator ({{provider}})",
              description:
                "Whether {{provider}} associates the address with automation.",
            },
          },
          media: {
            status: {
              name: "{{service}} availability",
              description: "Availability result reported for {{service}}.",
            },
            region: {
              name: "{{service}} region",
              description: "Service region reported for {{service}}.",
            },
            type: {
              name: "{{service}} result type",
              description: "Result classification reported for {{service}}.",
            },
          },
          mail: {
            connectivity: {
              name: "{{service}} connectivity",
              description: "Outbound mail connectivity result for {{service}}.",
            },
            dnsTotal: {
              name: "DNS blocklists checked",
              description: "Total number of DNS blocklists checked.",
            },
            dnsClean: {
              name: "Clean DNS blocklists",
              description:
                "Number of DNS blocklists that did not list the address.",
            },
            dnsMarked: {
              name: "Marked DNS blocklists",
              description: "Number of DNS blocklists that marked the address.",
            },
            dnsBlacklisted: {
              name: "Blacklisted DNS blocklists",
              description:
                "Number of DNS blocklists that blacklisted the address.",
            },
          },
        },
      },
      raw: {
        title: "IPQuality JSON",
        detail: "Public-IP sequence {{sequence}}, observed {{value}}",
        wrap: "Toggle line wrapping",
        copy: "Copy JSON",
        copied: "Copied",
        download: "Download JSON",
      },
    },
    historySettings: {
      title: "History and storage",
      detail:
        "Manage retention, inspect logical and physical usage, and clean observed data.",
      retry: "Retry",
      loadFailed: "History state could not be loaded",
      usage: {
        title: "Storage usage",
        detail:
          "Logical usage drives retention. Physical usage includes the SQLite database, WAL, and shared-memory files.",
        logical: "Logical usage",
        protected: "Protected usage",
        records: "History records",
        physical: "Physical usage",
        overBudget: "History usage exceeds the configured limit",
        overage:
          "Usage is {{value}} over budget. Stars, current snapshots, or active runs may prevent further cleanup.",
      },
      retention: {
        title: "Retention policy",
        detail:
          "Starred snapshots, current snapshots, current format state, and active runs are never removed automatically.",
        mode: "Policy",
        indefinite: "Keep indefinitely",
        age: "Keep by age",
        size: "Keep by logical usage",
        days: "Retention days",
        mib: "Logical limit (MiB)",
        updated: "Last updated: {{value}}",
        save: "Save and apply",
        invalid: "The retention policy values are invalid.",
      },
      cleanup: {
        action: "Clean now",
        lastRun: "Last cleanup",
        never: "Not yet run",
        lastDeleted: "Last deleted items",
        failed: "The last cleanup failed",
      },
      state: {
        title: "History state",
        detail:
          "The generation prevents offline Agent queues from restoring intentionally cleared history.",
        generation: "Current generation",
        resetAt: "Last cleared",
        never: "Never cleared",
      },
      feedback: {
        saved: "The retention policy was saved and applied.",
        cleaned: "Cleanup completed and removed {{count}} items.",
        cleared: "Observed history was cleared.",
      },
      danger: {
        title: "Clear observed history",
        detail:
          "This removes address events, probe runs, public-IP executions, snapshots, and history gaps. Node, public-IP settings, hidden paths, proxy, account, and task configuration remains.",
        action: "Clear history",
        confirmTitle: "Clear all observed history?",
        confirmDetail:
          "This action cannot be undone. Agents will discard queued data from the old generation; no complete probe starts automatically.",
        confirm: "Clear all history",
      },
    },
    notifications: {
      section: "Automation",
      title: "Notifications",
      detail:
        "Route address, probe, history-gap, and format events to configured senders.",
      refresh: "Refresh",
      retry: "Retry",
      loadFailed: "Notification settings could not be loaded",
      edit: "Edit",
      delete: "Delete",
      save: "Save",
      name: "Name",
      enabled: "Enabled",
      disabled: "Disabled",
      none: "None",
      tabs: {
        label: "Notification views",
        senders: "Senders",
        rules: "Rules",
        deliveries: "Delivery history",
      },
      feedback: {
        saved: "Notification settings saved.",
        deleted: "Notification setting deleted.",
        testQueued: "Test delivery queued through the normal delivery path.",
      },
      senderKind: {
        telegram: "Telegram",
        webhook: "Webhook",
        javascript: "JavaScript",
      },
      senders: {
        title: "Notification senders",
        detail:
          "Configure Telegram, generic Webhook, or isolated JavaScript delivery.",
        add: "Add sender",
        empty: "No notification senders are configured.",
        test: "Send test",
        deleteTitle: "Delete this notification sender?",
        deleteDetail:
          "A sender referenced by a rule or active delivery cannot be deleted.",
        edit: "Edit sender",
        create: "Create sender",
        formDetail:
          "Credentials are encrypted at rest. Hidden Telegram and Webhook credentials stay unchanged unless replaced.",
        kind: "Sender type",
        chatId: "Chat ID",
        token: "Bot token",
        keepToken: "Leave blank to keep the configured token.",
        url: "Webhook URL",
        replaceHeaders: "Replace configured headers",
        configuredHeaders: "Configured header names: {{value}}",
        headers: "HTTP headers",
        headersPlaceholder: "Authorization: Bearer token",
        invalidHeaders: "Enter one valid Name: value header per line.",
        source: "JavaScript source",
        sourceDetail:
          "Use ipchronicle.event, title, body, and synchronous ipchronicle.http.request().",
        telegramDetail: "Telegram chat {{value}}",
        javascriptDetail: "Runs in a fresh isolated worker for each delivery.",
      },
      rules: {
        title: "Matching rules",
        detail:
          "Match an event and optionally narrow it to a field, node, and public IP.",
        add: "Add rule",
        empty: "No notification rules are configured.",
        deleteTitle: "Delete this notification rule?",
        deleteDetail:
          "Existing delivery history remains, but future events will no longer match this rule.",
        edit: "Edit rule",
        create: "Create rule",
        formDetail:
          "Current enabled rules are evaluated when each durable event is processed.",
        sender: "Sender",
        event: "Event",
        field: "Known field ID",
        fieldPlaceholder: "IPQuality.ipinfo.CountryCode",
        node: "Node",
        egress: "Public IP",
        allNodes: "All nodes",
        allEgresses: "All public IPs on this node",
        addressUnavailable: "Public IP unavailable",
        senderRequired: "Create a sender before saving a notification rule.",
      },
      eventType: {
        "probe-field-change": "Probe field changed",
        "address-change": "Public address changed",
        "address-check-failure": "Address check failed",
        "address-check-recovery": "Address check recovered",
        "probe-failure": "Complete probe failed",
        "probe-recovery": "Complete probe recovered",
        "address-gap": "Address history gap",
        "probe-gap": "Probe history gap",
        "format-mismatch": "Probe format mismatch",
        "format-changed": "Probe format mismatch changed",
        "format-recovery": "Probe format recovered",
      },
      deliveries: {
        title: "Delivery history",
        detail:
          "Review active work, attempts, terminal outcomes, and bounded error codes.",
        sender: "Sender",
        allSenders: "All senders",
        status: "Status",
        allStatuses: "All statuses",
        loadFailed: "Notification delivery history could not be loaded",
        empty: "No deliveries match these filters.",
        event: "Event",
        attempts: "Attempts",
        created: "Created",
        error: "Terminal error",
        test: "Test delivery",
        page: "Page {{page}} of {{total}}",
        previous: "Previous",
        next: "Next",
      },
      deliveryStatus: {
        pending: "Pending",
        running: "Running",
        retrying: "Retrying",
        succeeded: "Succeeded",
        failed: "Failed",
      },
    },
    errors: {
      actionFailed: "The action could not be completed.",
      invalid_request: "The submitted values are invalid.",
      invalid_credentials: "The username, password, or code is incorrect.",
      totp_required: "Enter the six-digit authentication code.",
      rate_limited:
        "Too many attempts. Try again in {{retryAfterSeconds}} seconds.",
      unauthenticated: "The administrator session has expired.",
      csrf_failed:
        "The security token is invalid. Reload the page and try again.",
      origin_not_allowed: "The request origin is not allowed.",
      current_password_invalid: "The current password is incorrect.",
      invalid_totp: "The authentication code is invalid or was already used.",
      totp_already_enabled: "TOTP is already enabled.",
      totp_not_enabled: "TOTP is not enabled.",
      totp_enrollment_not_started:
        "Start TOTP enrollment before confirming it.",
      no_account_change: "Change the username or enter a new password.",
      registration_key_not_initialized:
        "Generate a registration key before enabling enrollment.",
      registration_key_invalid: "The registration key is invalid.",
      registration_disabled: "Automatic Agent enrollment is disabled.",
      agent_unauthenticated: "The Agent credential is invalid.",
      agent_revoked: "The Agent credential has been revoked.",
      node_not_found: "The node does not exist or has already been deleted.",
      node_revoked: "A node with a revoked Agent credential cannot be enabled.",
      node_deletion_pending:
        "The node is being permanently deleted and cannot accept other changes.",
      node_sync_unsupported:
        "The current Agent version does not support temporary sync.",
      sync_session_unavailable:
        "The temporary sync session has ended or is unavailable.",
      network_inventory_unavailable:
        "The node has not reported a valid network inventory.",
      invalid_egress_candidate: "This discovery path is currently unavailable.",
      egress_already_exists: "This proxy discovery path already exists.",
      egress_limit_reached:
        "This node has reached the maximum of 64 proxy discovery paths.",
      egress_not_found:
        "The proxy discovery path does not exist or was deleted.",
      egress_deletion_pending:
        "The proxy discovery path is being deleted and cannot accept other changes.",
      invalid_network_proxy: "The network proxy settings are invalid.",
      network_proxy_not_found:
        "The network proxy does not exist or was deleted.",
      network_proxy_already_exists:
        "A network proxy with this name already exists.",
      network_proxy_limit_reached:
        "The installation has reached the maximum of 64 network proxies.",
      network_proxy_in_use:
        "This proxy is still referenced by a proxy discovery path and cannot be deleted.",
      invalid_observation_settings:
        "Use 2 to 8 valid HTTP or HTTPS URLs with distinct hosts for each address family.",
      invalid_probe_settings: "The Cron expression or time zone is invalid.",
      node_offline: "The node is offline and cannot receive an immediate task.",
      probe_task_slot_occupied:
        "An immediate task is already waiting or running for this node.",
      probe_already_running:
        "A complete-probe run is already active on this node.",
      probe_paused_low_memory:
        "Complete probes are paused because this node reported less than 256 MiB of memory.",
      no_enabled_egress: "This node has no enabled public IP.",
      probe_run_not_found:
        "The complete-probe run does not exist or was removed.",
      probe_snapshot_not_found:
        "The report snapshot does not exist or was removed.",
      snapshot_egress_mismatch: "The snapshots belong to different public IPs.",
      invalid_notification_sender:
        "The notification sender settings are invalid.",
      notification_sender_not_found:
        "The notification sender does not exist or was deleted.",
      notification_sender_name_in_use:
        "A notification sender with this name already exists.",
      notification_sender_in_use:
        "This notification sender is still referenced by a rule.",
      notification_sender_active:
        "This notification sender still has an active delivery.",
      invalid_notification_rule: "The notification rule settings are invalid.",
      notification_rule_not_found:
        "The notification rule does not exist or was deleted.",
      notification_rule_name_in_use:
        "A notification rule with this name already exists.",
      invalid_notification_delivery_query:
        "The notification delivery filters are invalid.",
      invalid_system_settings: "The external address is invalid.",
      internal_error: "The center could not complete the request.",
    },
  },
} as const;
