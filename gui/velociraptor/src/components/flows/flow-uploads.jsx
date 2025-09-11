import React from 'react';
import PropTypes from 'prop-types';
import Button from 'react-bootstrap/Button';
import VeloPagedTable from '../core/paged-table.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import T from '../i8n/i8n.jsx';
import ToolTip from '../widgets/tooltip.jsx';
import PreviewUpload from '../widgets/preview_uploads.jsx';
import api from '../core/api-service.jsx';
import UserConfig from '../core/user.jsx';
import VeloTimestamp from "../utils/time.jsx";

// Older collections had the upload includes the full filestore path
// to the file, but this is un necessary because the file must reside
// int he client's upload directory. Handle both cases here.
const normalizeComponentList = (components, client_id, flow_id)=>{
    if (!components) {
        return components;
    }

    if (components[0] === "clients") {
        return components;
    }

    return ["clients", client_id, "collections", flow_id].concat(components);
};


export default class FlowUploads extends React.Component {
    static contextType = UserConfig;

    static propTypes = {
        flow: PropTypes.object,
    };

    render() {
        let flow_id = this.props.flow && this.props.flow.session_id;

        let renderers = {
            Timestamp: (cell, row) => {
                return <VeloTimestamp usec={cell}/>;
            },

            Preview: (cell, row, rowIndex) => {
                let client_id = this.props.flow && this.props.flow.client_id;
                let flow_id = this.props.flow && this.props.flow.session_id;
                let components = normalizeComponentList(
                    row._Components, client_id, flow_id);
                let padding = row.vfs_path && row.vfs_path.endsWith(".idx");
                return <PreviewUpload
                         env={{client_id: client_id, flow_id: flow_id}}
                         upload={{Path: row.vfs_path,
                                  Timestamp: row.started,
                                  Padding: padding,
                                  Size: row.uploaded_size || row.file_size,
                                  Components: components}} />;
            },

            // Let users directly download the file without having to
            // make a zip file.
            vfs_path: (cell, row, rowIndex) => {
                let filename = cell;
                let no_password = !this.context.traits.default_password;

                if (filename.endsWith(".idx")) {
                    filename = filename.slice(0, -4);
                    return <>
                             <ToolTip tooltip={T("Download padded file.")}
                                      key={rowIndex}>
                               <Button as="a"
                                       className="flow-file-download-button"
                                       target="_blank"
                                       href={api.href("/api/v1/DownloadVFSFile", {
                                           client_id: this.props.flow.client_id,
                                           fs_components: normalizeComponentList(
                                               row._Components, this.props.flow.client_id,
                                               this.props.flow.session_id),
                                           padding: true,
                                           zip: !no_password,
                                           vfs_path: filename})}>
                                 {filename} &nbsp;&nbsp;
                                 <FontAwesomeIcon icon="expand"/>
                                 { !no_password &&
                                   <div className="velo-icon">
                                     <FontAwesomeIcon icon="lock" />
                                   </div>}
                               </Button>
                             </ToolTip>
                           </>;
                }

                return <ToolTip tooltip={T("Download file.")}
                                key={rowIndex}>
                         <Button as="a"
                                 className="flow-file-download-button"
                                 target="_blank"
                                 href={api.href("/api/v1/DownloadVFSFile", {
                                     client_id: this.props.flow.client_id,
                                     fs_components: normalizeComponentList(
                                         row._Components, this.props.flow.client_id,
                                         this.props.flow.session_id),
                                     padding: false,
                                     zip: !no_password,
                                     vfs_path: filename})}>
                           { !no_password &&
                             <div className="velo-icon">
                               <FontAwesomeIcon icon="lock" />
                             </div>}
                           {filename}
                         </Button>
                       </ToolTip>;
            },
        };

        return (
            <VeloPagedTable
              className="col-12"
              renderers={renderers}
              extra_columns={["Preview"]}
              params={{
                  client_id: this.props.flow.client_id,
                  flow_id: this.props.flow.session_id,
                  type: "uploads",
              }}
              name={"FlowUploads" + flow_id}
            />
        );
    }
};
