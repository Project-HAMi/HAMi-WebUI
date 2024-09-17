<template>
  <div>
    <div
      id="modal-danger"
      ref="modal-danger"
      class="modal modal-blur"
      tabindex="-1"
      aria-modal="true"
      role="dialog"
    >
      <div class="modal-dialog modal-sm modal-dialog-centered" role="document">
        <div class="modal-content">
          <button
            type="button"
            class="btn-close"
            aria-label="Close"
            @click.prevent="onCancel()"
          />
          <div
            :class="`${
              confirmObj.type === 'danger'
                ? 'modal-status bg-danger'
                : 'modal-status bg-success'
            }`"
          />
          <div class="modal-body text-center py-4">
            <svg-icon
              v-if="confirmObj.type === 'danger'"
              icon="textDanger"
            ></svg-icon>
            <svg-icon
              v-if="confirmObj.type === 'success'"
              icon="textGreen"
            ></svg-icon>
            <div class="text-secondary">{{ confirmText }}</div>
          </div>
          <div class="modal-footer">
            <div class="w-100">
              <div class="row">
                <div class="col">
                  <a href="#" class="btn w-100" @click.prevent="onCancel()">
                    {{ confirmObj.cancelButtonText }}
                  </a>
                </div>
                <div class="col">
                  <button
                    :class="`${
                      confirmObj.type === 'danger'
                        ? 'btn w-100 btn-danger'
                        : 'btn w-100 btn-success'
                    }`"
                    @click.prevent="deleteMenu()"
                  >
                    {{ confirmObj.confirmButtonText }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { Modal } from 'bootstrap';

export default {
  //   props: {
  //     confirmText: {
  //       type: String,
  //       default: '',
  //     },
  //     confirmObj: {
  //       type: Object,
  //       default: () => {
  //         return {
  //           confirmButtonText: '确定',
  //           cancelButtonText: '取消',
  //           type: '',
  //         };
  //       },
  //     },
  //     fn: {
  //       type: Function,
  //       default: null,
  //     },
  //   },
  data() {
    return {
      removeModal: null,
      fn: null,
      confirmObj: {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: '',
      },
      confirmText: '',
    };
  },
  mounted() {
    this.removeModal = new Modal('#modal-danger');
  },
  methods: {
    showConfirm(confirmText, confirmObj, fn) {
      this.removeModal.show();
      this.confirmObj = confirmObj;
      this.fn = fn;
      this.confirmText = confirmText;
    },
    onCancel() {
      this.removeModal.hide();
    },
    deleteMenu() {
      new Promise((resovle, reject) => {
        this.fn();
        resovle();
      }).then((res) => {
        this.removeModal.hide();
      });
    },
  },
};
</script>

<style></style>
