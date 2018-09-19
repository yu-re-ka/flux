/*
 * Headers
*/

#include <stdint.h>
#include <stddef.h>
#include <stdbool.h>
#define CL_USE_DEPRECATED_OPENCL_1_2_APIS
#ifdef __APPLE__
#include <OpenCL/cl.h>
#else
#include <CL/cl.h>
#endif


/*
 * Initialisation
*/

int futhark_get_num_sizes(void);
const char *futhark_get_size_name(int);
const char *futhark_get_size_class(int);
const char *futhark_get_size_entry(int);
struct futhark_context_config ;
struct futhark_context_config *futhark_context_config_new(void);
void futhark_context_config_free(struct futhark_context_config *cfg);
void futhark_context_config_set_debugging(struct futhark_context_config *cfg,
                                          int flag);
void futhark_context_config_set_logging(struct futhark_context_config *cfg,
                                        int flag);
void futhark_context_config_set_device(struct futhark_context_config *cfg, const
                                       char *s);
void futhark_context_config_set_platform(struct futhark_context_config *cfg,
                                         const char *s);
void futhark_context_config_dump_program_to(struct futhark_context_config *cfg,
                                            const char *path);
void
futhark_context_config_load_program_from(struct futhark_context_config *cfg,
                                         const char *path);
void
futhark_context_config_set_default_group_size(struct futhark_context_config *cfg,
                                              int size);
void
futhark_context_config_set_default_num_groups(struct futhark_context_config *cfg,
                                              int num);
void
futhark_context_config_set_default_tile_size(struct futhark_context_config *cfg,
                                             int num);
void
futhark_context_config_set_default_threshold(struct futhark_context_config *cfg,
                                             int num);
int futhark_context_config_set_size(struct futhark_context_config *cfg, const
                                    char *size_name, size_t size_value);
struct futhark_context ;
struct futhark_context *futhark_context_new(struct futhark_context_config *cfg);
struct futhark_context
*futhark_context_new_with_command_queue(struct futhark_context_config *cfg,
                                        cl_command_queue queue);
void futhark_context_free(struct futhark_context *ctx);
int futhark_context_sync(struct futhark_context *ctx);
char *futhark_context_get_error(struct futhark_context *ctx);
int futhark_context_clear_caches(struct futhark_context *ctx);
cl_command_queue futhark_context_get_command_queue(struct futhark_context *ctx);

/*
 * Arrays
*/

struct f64_1d ;
struct futhark_f64_1d *futhark_new_f64_1d(struct futhark_context *ctx,
                                          double *data, int dim0);
struct futhark_f64_1d *futhark_new_raw_f64_1d(struct futhark_context *ctx,
                                              cl_mem data, int offset,
                                              int dim0);
int futhark_free_f64_1d(struct futhark_context *ctx,
                        struct futhark_f64_1d *arr);
int futhark_values_f64_1d(struct futhark_context *ctx,
                          struct futhark_f64_1d *arr, double *data);
cl_mem futhark_values_raw_f64_1d(struct futhark_context *ctx,
                                 struct futhark_f64_1d *arr);
int64_t *futhark_shape_f64_1d(struct futhark_context *ctx,
                              struct futhark_f64_1d *arr);

/*
 * Opaque values
*/


/*
 * Entry points
*/

int futhark_entry_sum(struct futhark_context *ctx, double *out0, const
                      struct futhark_f64_1d *in0);
int futhark_entry_mean(struct futhark_context *ctx, double *out0, const
                       struct futhark_f64_1d *in0);
int futhark_entry_variance(struct futhark_context *ctx, double *out0, const
                           struct futhark_f64_1d *in0);
int futhark_entry_stddev(struct futhark_context *ctx, double *out0, const
                         struct futhark_f64_1d *in0);

/*
 * Miscellaneous
*/

void futhark_debugging_report(struct futhark_context *ctx);
