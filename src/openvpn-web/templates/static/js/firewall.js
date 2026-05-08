tables.firewall = {
  scrollY: 'calc(100vh - 300px)',
  scrollCollapse: true,
  paging: false,
  autoWidth: false,
  responsive: false,
  columns: [
    {
      title: 'ID',
      data: 'id',
    },
    {
      title: '源地址',
      data: 'sip',
      render: (data, type, row) => {
        const html = [];
        if (data) html.push(...data.split(','));

        if (row.sg.length > 0) {
          html.push(...row.sg.sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt)).map((g) => g.name));
        }

        return html.join('<br />');
      },
    },
    {
      title: '目的地址',
      data: 'dip',
      render: (data, type, row) => {
        const html = [];
        if (data) html.push(...data.split(','));

        if (row.dg.length > 0) {
          html.push(...row.dg.sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt)).map((g) => g.name));
        }

        return html.join('<br />');
      },
    },
    {
      title: '策略',
      data: 'policy',
    },
    {
      title: '状态',
      data: 'status',
      render: (data, type, row) => {
        return data === true
          ? '<span class="badge text-bg-success">启用</span>'
          : '<span class="badge text-bg-danger">禁用</span>';
      },
    },
    { title: '备注', data: 'comment', className: 'dt-center w-max-200 text-truncate' },
    {
      title: '创建时间',
      data: 'createdAt',
      render: (data, type, row) => dayjs(data).format('YYYY-MM-DD HH:mm:ss'),
    },
    {
      title: '操作',
      data: null,
      orderable: false,
      searchable: false,
      render: (data, type, row) => {
        const html = `
          <div class="d-flex gap-2 justify-content-center align-items-center">
            <button class="btn btn-link text-decoration-none p-0" type="button" id="editFirewall">编辑</button>
            ${
              row.status === true
                ? '<button class="btn btn-link text-decoration-none p-0" type="button" id="disableFirewall">禁用</button>'
                : '<button class="btn btn-link text-decoration-none p-0" type="button" id="enableFirewall">启用</button>'
            }
            <button class="btn btn-link text-decoration-none p-0 btn-delete" data-bs-toggle="popover" data-delete-type="firewall" data-delete-name="${
              row.id
            }">删除</button>
          </div>
          `;
        return html;
      },
    },
  ],
  order: [[6, 'desc']],
  buttons: {
    dom: {
      button: { className: 'btn btn-sm' },
    },
    buttons: [
      {
        text: '添加',
        className: 'btn-primary',
        action: () => openFirewallModal('#addFirewallModal', null),
      },
    ],
  },
  dom:
    "<'d-flex justify-content-between align-items-center'fB>" +
    "<'row'<'col-sm-12'tr>>" +
    "<'d-flex justify-content-between align-items-center'lip>",
  drawCallback: function (settings) {
    $('#vtable .btn-delete').popover('dispose');
    $('#vtable .btn-delete').popover({
      container: 'body',
      placement: 'top',
      html: true,
      sanitize: false,
      trigger: 'click',
      title: '提示',
      content: function (e) {
        const name = $(e).data('delete-name');
        return `
            <div>
              <p>确认删除 <strong>${name}</strong> 吗？</p>
              <div class="d-flex justify-content-center">
                <button class="btn btn-secondary btn-sm me-2 btn-popover-cancel">取消</button>
                <button class="btn btn-primary btn-sm btn-popover-confirm">确认</button>
              </div>
            </div>
          `;
      },
    });
  },
  ajax: function (data, callback, settings) {
    request.get('/ovpn/firewall').then((data) => callback({ data }));
  },
};

function isValidIPOrRange(val) {
  if (typeof val !== 'string') return false;
  const input = val.trim();

  const ipv4Part = '(25[0-5]|2[0-4]\\d|1\\d\\d|[1-9]\\d|\\d)';
  const ipv4Regex = new RegExp(`^(${ipv4Part}\\.){3}${ipv4Part}$`);
  const ipv6Regex =
    /^(([0-9a-fA-F]{1,4}:){7,7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:)|fe80:(:[0-9a-fA-F]{0,4}){0,4}%[0-9a-zA-Z]{1,}|::(ffff(:0{1,4}){0,1}:){0,1}((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])|([0-9a-fA-F]{1,4}:){1,4}:((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9]))$/;

  const ipv4ToBigInt = (ip) => {
    return ip.split('.').reduce((acc, octet) => (acc << 8n) + BigInt(octet), 0n);
  };

  const ipv6ToBigInt = (ip) => {
    let fullIp = ip;
    if (ip.includes('::')) {
      const parts = ip.split('::');
      const left = parts[0] ? parts[0].split(':') : [];
      const right = parts[1] ? parts[1].split(':') : [];
      const missing = new Array(8 - (left.length + right.length)).fill('0');
      fullIp = [...left, ...missing, ...right].join(':');
    }
    return fullIp.split(':').reduce((acc, hex) => (acc << 16n) + BigInt(parseInt(hex || '0', 16)), 0n);
  };

  if (input.includes('/')) {
    const [ip, maskStr] = input.split('/');
    const mask = parseInt(maskStr, 10);
    if (isNaN(mask)) return false;
    if (ipv4Regex.test(ip)) return mask >= 0 && mask <= 32;
    if (ipv6Regex.test(ip)) return mask >= 0 && mask <= 128;
    return false;
  }

  if (input.includes('-')) {
    const [start, end] = input.split('-').map((s) => s.trim());
    if (ipv4Regex.test(start) && ipv4Regex.test(end)) {
      return ipv4ToBigInt(start) <= ipv4ToBigInt(end);
    }
    if (ipv6Regex.test(start) && ipv6Regex.test(end)) {
      return ipv6ToBigInt(start) <= ipv6ToBigInt(end);
    }
    return false;
  }

  return ipv4Regex.test(input) || ipv6Regex.test(input);
}

// 用户组树形菜单
const treeData = new Map();
const nodeKey = (type, id) => `${type}:${id}`;

const resetTreeData = (m, d) => {
  treeData[`${m}_${d}`] = {
    modal: m,
    side: d,
    groups: [],
    inputs: [],
    chosen: new Set(),
    leftSel: new Set(),
    rightSel: new Set(),
    leftExp: new Set(),
    rightExp: new Set(),
  };
};

function buildTree(data, visibleIds) {
  const pathNodeIds = new Set();
  const nodeMap = new Map(data.map((i) => [i.id, i]));

  visibleIds.forEach((id) => {
    let currentNode = nodeMap.get(id);
    while (currentNode) {
      pathNodeIds.add(currentNode.id);
      currentNode = currentNode.parent_id == null ? null : nodeMap.get(currentNode.parent_id);
    }
  });

  const buildData = (parentId = null, depth = 0) =>
    data
      .filter((i) => i.parent_id === parentId && pathNodeIds.has(i.id))
      .map((i) => ({
        ...i,
        depth,
        isPhantom: !visibleIds.has(i.id),
        children: buildData(i.id, depth + 1),
      }));

  return buildData();
}

function makeNode(node) {
  const cls = ['firewall-tree-item'];
  const toggleCls = node.hasChildren ? (node.isExpanded ? 'expanded' : '') : 'hidden';
  const toggleIcon = node.hasChildren ? '<i class="fas fa-chevron-right"></i>' : '';

  if (node.isPhantom) cls.push('phantom');
  if (node.selected) cls.push('selected');

  const li = `
  <li>
    <div
      class="${cls.join(' ')}"
      data-node-type="${node.type}"
      data-node-id="${node.id}"
      data-phantom="${node.isPhantom ? '1' : '0'}"
    >
      <span class="tree-toggle ${toggleCls}">${toggleIcon}</span>
      <i class="fas ${node.icon}"></i>
      <span class="tree-name" title="${node.name}">${node.name}</span>
    </div>
  </li>
  `;

  return $(li);
}

function renderTreeNodes(nodes, parent, expSet, selSet) {
  nodes.forEach((node) => {
    const hasChildren = node.children.length > 0;
    const isExpanded = expSet.has(node.id);
    const folder = hasChildren && isExpanded ? 'fa-folder-open' : 'fa-folder';

    const li = makeNode({
      id: node.id,
      name: node.name,
      type: 'group',
      icon: `${folder} text-warning`,
      isPhantom: node.isPhantom,
      selected: !node.isPhantom && selSet.has(nodeKey('group', node.id)),
      hasChildren,
      isExpanded,
    });

    if (hasChildren) {
      const ul = $('<ul class="firewall-tree-list"></ul>').toggle(isExpanded);
      renderTreeNodes(node.children, ul, expSet, selSet);
      li.append(ul);
    }

    parent.append(li);
  });
}

function renderTree(modal, selBox, isLeft) {
  const ctx = treeData[`${modal}_${selBox}`];
  // 不渲染目标地址用户组树形菜单（openvpn启用client-to-client不受防火墙限制）
  if (!ctx || (isLeft && selBox === 'd')) return;

  const box = $(`${modal} .firewall-tree[data-name="${selBox + (isLeft ? 'ug' : 'g')}"]`);
  if (!box.length) return;

  const visibleIds = isLeft
    ? new Set(ctx.groups.filter((g) => !ctx.chosen.has(g.id)).map((g) => g.id))
    : new Set([...ctx.chosen]);
  const tree = buildTree(ctx.groups, visibleIds);
  const expSet = isLeft ? ctx.leftExp : ctx.rightExp;
  const selSet = isLeft ? ctx.leftSel : ctx.rightSel;
  const hasInputs = !isLeft && ctx.inputs.length > 0;
  const hasTree = tree.length > 0;

  ctx.chosen.forEach((id) => {
    rightExpand(ctx, id);
  });

  box.empty();
  if (!hasInputs && !hasTree) {
    return;
  }

  if (hasInputs) {
    if (hasTree) box.append('<div class="firewall-tree-section-title">IP</div>');

    const ul = $('<ul class="firewall-tree-list"></ul>');
    ctx.inputs.forEach((ip) =>
      ul.append(
        makeNode({
          type: 'input',
          id: ip.value,
          name: ip.value,
          icon: 'fa-globe text-info',
          selected: selSet.has(nodeKey('input', ip.value)),
        })
      )
    );

    box.append(ul);
  }

  if (hasTree) {
    if (hasInputs) box.append('<div class="firewall-tree-section-title">用户组</div>');
    const ul = $('<ul class="firewall-tree-list"></ul>');
    renderTreeNodes(tree, ul, expSet, selSet);
    box.append(ul);
  }
}

function clearSelectionExcept(modal, except) {
  ['s', 'd'].forEach((selBox) => {
    const ctx = treeData[`${modal}_${selBox}`];
    if (!ctx) return;

    [
      { sel: ctx.leftSel, isLeft: true },
      { sel: ctx.rightSel, isLeft: false },
    ].forEach(({ sel, isLeft }) => {
      const skip = except && except.selBox === selBox && except.isLeft === isLeft;
      if (!skip && sel.size) {
        sel.clear();
        renderTree(modal, selBox, isLeft);
      }
    });
  });
}

function rightExpand(ctx, id) {
  const map = new Map(ctx.groups.map((g) => [g.id, g]));
  let cg = map.get(id);
  while (cg) {
    ctx.rightExp.add(cg.id);
    cg = cg.parent_id == null ? null : map.get(cg.parent_id);
  }
}

function openFirewallModal(m, data) {
  resetTreeData(m, 's');
  resetTreeData(m, 'd');

  $(`${m} form`).trigger('reset');
  $(m).find("input[name='sip'], input[name='dip']").removeClass('border border-danger');

  const isEdit = !!data;
  if (isEdit) {
    $(`${m} input[name='id']`).val(data.id);
    $(`${m} select[name='policy']`).val(data.policy);
    $(`${m} textarea[name='comment']`).val(data.comment);
    $(`${m} input[name='status']`).prop('checked', data.status);
  }

  request.get('/ovpn/group').then((groups) => {
    ['s', 'd'].forEach((i) => {
      const key = `${m}_${i}`;
      const ctx = treeData[key];
      ctx.groups = groups;
      ctx.leftExp = new Set(ctx.groups.filter((g) => g.parent_id === null).map((g) => g.id));

      if (isEdit) {
        data[`${i}g`].forEach((g) => {
          ctx.chosen.add(g.id);
          if (!ctx.groups.some((x) => x.id === g.id)) ctx.groups.push(g);
        });

        const ips = data[`${i}ip`];
        if (ips) ips.split(',').forEach((ip) => ip && ctx.inputs.push({ value: ip }));
      }

      renderTree(m, i, true);
      renderTree(m, i, false);
    });

    $(m).modal('show');
  });
}

// 事件绑定
['#addFirewallModal', '#editFirewallModal'].forEach((m) => {
  // 保存按钮
  $(`${m} form`).submit(function (e) {
    e.preventDefault();

    const sip = treeData[`${m}_s`].inputs.map((i) => i.value);
    const sg = [...treeData[`${m}_s`].chosen].map(String);
    const dip = treeData[`${m}_d`].inputs.map((i) => i.value);
    const dg = [...treeData[`${m}_d`].chosen].map(String);

    if (sip.length + sg.length === 0 || dip.length + dg.length === 0) {
      message.error('源地址或目的地址为空');
      return;
    }

    const data = {
      sip,
      dip,
      sg,
      dg,
      policy: $(`${m} select[name='policy']`).val(),
      comment: $(`${m} textarea[name='comment']`).val(),
    };

    const isEdit = m === '#editFirewallModal';
    if (isEdit) {
      data.id = $(`${m} input[name='id']`).val();
      data.status = $(`${m} input[name='status']`).prop('checked');
    }

    request[isEdit ? 'patch' : 'post']('/ovpn/firewall', data).then((res) => {
      message.success(res.message);
      vtable.ajax.reload(null, false);
      $(m).modal('hide');
    });
  });

  ['s', 'd'].forEach((i) => {
    // 加入按钮
    $(`${m} button[name="${i}addBtn"]`).click(function () {
      const ctx = treeData[`${m}_${i}`];
      if (!ctx) return;

      const ip = $(`${m} input[name='${i}ip']`);
      if (ip.val() && isValidIPOrRange(ip.val())) {
        if (!ctx || ctx.inputs.some((i) => i.value === ip.val())) return;
        ctx.inputs.push({ value: ip.val() });
        ip.val('');
      }

      ctx.leftSel.forEach((i) => {
        const [type, id] = i.split(':');
        if (type === 'group') {
          ctx.chosen.add(Number(id));
        }
      });

      ctx.leftSel.clear();

      renderTree(m, i, true);
      renderTree(m, i, false);
    });

    // 移除按钮
    $(`${m} button[name="${i}delBtn"]`).click(function () {
      const ctx = treeData[`${m}_${i}`];
      if (!ctx) return;

      ctx.rightSel.forEach((i) => {
        const [type, id] = i.split(':');
        if (type === 'group') {
          ctx.chosen.delete(Number(id));
        } else if (type === 'input') {
          ctx.inputs = ctx.inputs.filter((i) => i.value !== id);
        }
      });

      ctx.rightSel.clear();

      renderTree(m, i, true);
      renderTree(m, i, false);
    });
  });

  // tree
  const treeItem = `${m} .firewall-tree .firewall-tree-item`;
  $(document)
    .on('click', treeItem, function (e) {
      e.stopPropagation();

      const name = $(this).closest('.firewall-tree').data('name');
      const isLeft = name.endsWith('ug');

      const ctx = treeData[`${m}_${name[0]}`];
      if (!ctx) return;

      const expSet = isLeft ? ctx.leftExp : ctx.rightExp;
      const selSet = isLeft ? ctx.leftSel : ctx.rightSel;
      const nodeType = $(this).attr('data-node-type');
      const rawId = $(this).attr('data-node-id');

      if ($(e.target).closest('.tree-toggle').length && nodeType === 'group') {
        const nid = Number(rawId);
        expSet.has(nid) ? expSet.delete(nid) : expSet.add(nid);
        renderTree(m, name[0], isLeft);
        return;
      }

      if ($(this).attr('data-phantom') === '1') return;

      clearSelectionExcept(m, { selBox: name[0], isLeft });

      const k = nodeKey(nodeType, rawId);
      if (!(e.ctrlKey || e.metaKey)) selSet.clear();
      selSet.has(k) ? selSet.delete(k) : selSet.add(k);
      renderTree(m, name[0], isLeft);
    })
    .on('click', m, function (e) {
      if ($(e.target).closest('.firewall-tree-item, button, input, select, textarea, label, a').length) return;
      clearSelectionExcept(m, null);
    });
});

// 添加规则
$("#addFirewallModal input[name='sip']").on('input', function () {
  const val = $(this).val();
  $(this).toggleClass('border border-danger', val !== '' && !isValidIPOrRange(val));
});

$("#addFirewallModal input[name='dip']").on('input', function () {
  const val = $(this).val();
  $(this).toggleClass('border border-danger', val !== '' && !isValidIPOrRange(val));
});

// 编辑防火墙规则
$(document).on('click', '#editFirewall', function () {
  const data = vtable.row($(this).parents('tr')).data();
  openFirewallModal('#editFirewallModal', data);
});

$("#editFirewallModal input[name='sip']").on('input', function () {
  const val = $(this).val();
  $(this).toggleClass('border border-danger', val !== '' && !isValidIPOrRange(val));
});

$("#editFirewallModal input[name='dip']").on('input', function () {
  const val = $(this).val();
  $(this).toggleClass('border border-danger', val !== '' && !isValidIPOrRange(val));
});

// 禁用规则
$(document).on('click', '#disableFirewall', function () {
  const data = vtable.row($(this).parents('tr')).data();

  request
    .patch('/ovpn/firewall', {
      ...data,
      status: false,
      sg: data.sg.map((i) => i.id).join(','),
      dg: data.dg.map((i) => i.id).join(','),
    })
    .then((data) => {
      message.success(data.message);
      vtable.ajax.reload(null, false);
    });
});

// 启用规则
$(document).on('click', '#enableFirewall', function () {
  const data = vtable.row($(this).parents('tr')).data();

  request
    .patch('/ovpn/firewall', {
      ...data,
      status: true,
      sg: data.sg.map((i) => i.id).join(','),
      dg: data.dg.map((i) => i.id).join(','),
    })
    .then((data) => {
      message.success(data.message);
      vtable.ajax.reload(null, false);
    });
});
