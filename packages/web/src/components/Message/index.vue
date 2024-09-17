<template>
  <div v-if="isShow" class="row">
    <div
      v-if="type === 'success'"
      class="alert alert-success alert-dismissible col-md-3"
      role="alert"
      style="
        position: fixed;
        z-index: 100;
        top: 20px;
        right: 10px;
        background-color: #fff;
      "
    >
      <div class="d-flex">
        <div>
          <svg
            xmlns="http://www.w3.org/2000/svg"
            class="icon alert-icon"
            width="24"
            height="24"
            viewBox="0 0 24 24"
            stroke-width="2"
            stroke="currentColor"
            fill="none"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path stroke="none" d="M0 0h24v24H0z" fill="none" />
            <path d="M5 12l5 5l10 -10" />
          </svg>
        </div>
        <div>
          <h4 class="alert-title">{{ msg }}</h4>
        </div>
      </div>
      <a class="btn-close" data-bs-dismiss="alert" aria-label="close" />
    </div>

    <div
      v-if="type === 'info'"
      class="alert alert-info alert-dismissible col-md-3"
      role="alert"
      style="
        position: fixed;
        z-index: 100;
        top: 20px;
        right: 10px;
        background-color: #fff;
      "
    >
      <div class="d-flex">
        <div>
          <svg
            xmlns="http://www.w3.org/2000/svg"
            class="icon alert-icon"
            width="24"
            height="24"
            viewBox="0 0 24 24"
            stroke-width="2"
            stroke="currentColor"
            fill="none"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path stroke="none" d="M0 0h24v24H0z" fill="none"></path>
            <circle cx="12" cy="12" r="9"></circle>
            <line x1="12" y1="8" x2="12.01" y2="8"></line>
            <polyline points="11 12 12 12 12 16 13 16"></polyline>
          </svg>
        </div>
        <div>
          <h4 class="alert-title">{{ msg }}</h4>
        </div>
      </div>
      <a class="btn-close" data-bs-dismiss="alert" aria-label="close"></a>
    </div>

    <div
      v-if="type === 'warning'"
      class="alert alert-warning alert-dismissible col-md-3"
      role="alert"
      style="
        position: fixed;
        z-index: 100;
        top: 20px;
        right: 10px;
        background: #fff;
      "
    >
      <div class="d-flex">
        <div>
          <svg
            xmlns="http://www.w3.org/2000/svg"
            class="icon alert-icon"
            width="24"
            height="24"
            viewBox="0 0 24 24"
            stroke-width="2"
            stroke="currentColor"
            fill="none"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path stroke="none" d="M0 0h24v24H0z" fill="none"></path>
            <path d="M12 9v2m0 4v.01"></path>
            <path
              d="M5 19h14a2 2 0 0 0 1.84 -2.75l-7.1 -12.25a2 2 0 0 0 -3.5 0l-7.1 12.25a2 2 0 0 0 1.75 2.75"
            ></path>
          </svg>
        </div>
        <div>
          <h4 class="alert-title">{{ msg }}</h4>
        </div>
      </div>
      <a class="btn-close" data-bs-dismiss="alert" aria-label="close"></a>
    </div>

    <div
      v-if="type === 'danger'"
      class="alert alert-danger alert-dismissible col-md-3"
      role="alert"
      style="
        position: fixed;
        z-index: 100;
        top: 20px;
        right: 10px;
        background: #fff;
      "
    >
      <div class="d-flex">
        <div>
          <svg
            xmlns="http://www.w3.org/2000/svg"
            class="icon alert-icon"
            width="24"
            height="24"
            viewBox="0 0 24 24"
            stroke-width="2"
            stroke="currentColor"
            fill="none"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path stroke="none" d="M0 0h24v24H0z" fill="none"></path>
            <circle cx="12" cy="12" r="9"></circle>
            <line x1="12" y1="8" x2="12" y2="12"></line>
            <line x1="12" y1="16" x2="12.01" y2="16"></line>
          </svg>
        </div>
        <div>
          <h4 class="alert-title">{{ msg }}</h4>
        </div>
      </div>
      <a class="btn-close" data-bs-dismiss="alert" aria-label="close"></a>
    </div>
  </div>
</template>

<script>
export default {
  // props: {
  //   type: {
  //     type: String,
  //     default: '',
  //   },
  //   msg: {
  //     type: String,
  //     default: '',
  //   },
  //   duration: {
  //     type: Number,
  //     default: 2000,
  //   },
  // },
  data() {
    return {
      isShow: false,
      type: '',
      msg: '',
    };
  },
  methods: {
    showMessage(type, msg) {
      this.isShow = true;
      this.type = type;
      this.msg = msg;
    },
    hideMessage() {
      this.isShow = false;
    },
  },
};
</script>

<style></style>
