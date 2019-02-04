/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

#include <windows.h>
#include <wchar.h>

typedef struct {
    wchar_t *filename;
    wchar_t *program_name;
    wchar_t *publisher_link;
    wchar_t *more_info_link;
    char *signer_cert_serial_number;
    char *issuer_name;
    char *subject_name;

    char *timestamp_issuer_name;
    char *timestamp_subject_name;
    char *timestamp;

    // Static strings - do not free.
    char *trusted;
} authenticode_data_struct;


// Populate the authenticode_data_struct with information from the filename.
int verify_file_authenticode(wchar_t *filename, authenticode_data_struct* result);

// Free any C allocated strings.
void free_authenticode_data_struct(authenticode_data_struct *data);
