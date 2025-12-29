!(function () {
  'use strict';

  // Message 类 - 用于显示消息提示
  class Message {
    constructor() {
      this.div = document.createElement('div');
      this.div.id = 'message';
      this.div.className = 'message';

      if (!document.getElementById('message')) {
        document.body.appendChild(this.div);
      }
    }

    open(type = 'info', content, duration, onClose) {
      duration = duration == 0 ? duration : (duration || 3) * 1000;

      const html = `
    <div class="message-notice">
      <div class="message-content message-${type}">
        <span class="message-anticon">
          ${this.icon}
        </span>
        <span>${content}</span>
      </div>
    </div>
    `;

      const $msgDiv = $(html).appendTo('#message');
      $msgDiv.removeClass('move-up-leave').addClass('move-up-enter');
      setTimeout(() => {
        $msgDiv.removeClass('move-up-enter');
      }, 300);
      if (duration != 0) {
        setTimeout(function () {
          $msgDiv.removeClass('move-up-enter').addClass('move-up-leave');
          setTimeout(() => {
            $msgDiv.removeClass('move-up-leave').remove();
          }, 300);
          if (onClose && typeof onClose === 'function') {
            onClose();
          }
        }, duration);
      }
    }

    info(content, duration, onClose) {
      this.icon = `
    <svg xmlns="http://www.w3.org/2000/svg" width="1em" height="1em" fill="currentColor" viewBox="0 0 16 16">
      <path d="M16 8A8 8 0 1 1 0 8a8 8 0 0 1 16 0zM8 4a.905.905 0 0 0-.9.995l.35 3.507a.552.552 0 0 0 1.1 0l.35-3.507A.905.905 0 0 0 8 4zm.002 6a1 1 0 1 0 0 2 1 1 0 0 0 0-2z"/>
    </svg>`;

      this.open('info', content, duration, onClose);
    }

    success(content, duration, onClose) {
      this.icon = `
    <svg xmlns="http://www.w3.org/2000/svg" width="1em" height="1em" fill="currentColor" viewBox="0 0 16 16">
      <path d="M16 8A8 8 0 1 1 0 8a8 8 0 0 1 16 0zm-3.97-3.03a.75.75 0 0 0-1.08.022L7.477 9.417 5.384 7.323a.75.75 0 0 0-1.06 1.06L6.97 11.03a.75.75 0 0 0 1.079-.02l3.992-4.99a.75.75 0 0 0-.01-1.05z"/>
    </svg>`;

      this.open('success', content, duration, onClose);
    }

    warning(content, duration, onClose) {
      this.icon = `
    <svg xmlns="http://www.w3.org/2000/svg" width="1em" height="1em" fill="currentColor" viewBox="0 0 16 16">
      <path d="M16 8A8 8 0 1 1 0 8a8 8 0 0 1 16 0zM8 4a.905.905 0 0 0-.9.995l.35 3.507a.552.552 0 0 0 1.1 0l.35-3.507A.905.905 0 0 0 8 4zm.002 6a1 1 0 1 0 0 2 1 1 0 0 0 0-2z"/>
    </svg>`;

      this.open('warning', content, duration, onClose);
    }

    error(content, duration, onClose) {
      this.icon = `
    <svg xmlns="http://www.w3.org/2000/svg" width="1em" height="1em" fill="currentColor" viewBox="0 0 16 16">
      <path d="M16 8A8 8 0 1 1 0 8a8 8 0 0 1 16 0zM5.354 4.646a.5.5 0 1 0-.708.708L7.293 8l-2.647 2.646a.5.5 0 0 0 .708.708L8 8.707l2.646 2.647a.5.5 0 0 0 .708-.708L8.707 8l2.647-2.646a.5.5 0 0 0-.708-.708L8 7.293 5.354 4.646z"/>
    </svg>
    `;

      this.open('error', content, duration, onClose);
    }
  }

  // Request 类 - 用于处理HTTP请求
  class Request {
    async request(method, url, data = null) {
      const options = {
        method,
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded; charset=UTF-8',
        },
        redirect: 'manual',
      };

      if (data) {
        options.body = new URLSearchParams(data);
      }

      try {
        const response = await fetch(url, options);

        if (response.type == 'opaqueredirect') {
          window.location.href = response.url;
          return;
        }

        const body = await response.json();
        if (!response.ok) {
          throw new Error(body?.message || response.text || response.statusText);
        }

        return body;
      } catch (error) {
        switch (true) {
          case error.message.includes('UNIQUE constraint failed: user.ip_addr'):
            message.error('IP已经使用');
            break;
          case error.message.includes('UNIQUE constraint failed: user.username'):
            message.error('用户名已存在');
            break;
          default:
            message.error(error.message);
        }

        throw error;
      }
    }

    get(url) {
      return this.request('GET', url);
    }

    put(url, data) {
      return this.request('PUT', url, data);
    }

    post(url, data) {
      return this.request('POST', url, data);
    }

    delete(url) {
      return this.request('DELETE', url);
    }

    patch(url, data) {
      return this.request('PATCH', url, data);
    }
  }

  const message = new Message();
  const request = new Request();

  window.message = message;
  window.request = request;
})();
