import _ from 'lodash';

import React from 'react';
import PropTypes from 'prop-types';
import Button from 'react-bootstrap/Button';
import axios from 'axios';
import VeloTable, { PrepareData } from '../core/table.js';
import T from '../i8n/i8n.js';
import Spinner from '../utils/spinner.js';
import CardDeck from 'react-bootstrap/CardDeck';
import Card from 'react-bootstrap/Card';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import OverlayTrigger from 'react-bootstrap/OverlayTrigger';
import Tooltip from 'react-bootstrap/Tooltip';

import api from '../core/api-service.js';
const MAX_ROWS_PER_TABLE = 500;

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
    static propTypes = {
        flow: PropTypes.object,
    };

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.fetchRows();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    componentDidUpdate(prevProps, prevState, snapshot) {
        let prev_flow_id = prevProps.flow && prevProps.flow.session_id;
        if (this.props.flow.session_id !== prev_flow_id) {
            this.fetchRows();
        }
    }

    state = {
        loading: false,
        pageData: {},
    }

    fetchRows = () => {
        let params = {
            client_id: this.props.flow.client_id,
            flow_id: this.props.flow.session_id,
            type: "uploads",
            start_row: 0,
            rows: MAX_ROWS_PER_TABLE,
        };

        this.source.cancel();
        this.source = axios.CancelToken.source();

        this.setState({loading: true});
        api.get("v1/GetTable", params, this.source.token).then((response) => {
            if (response.cancel) return;

            let prepared_data = PrepareData(response.data);
            // Translate the columns
            let headers = {};
            _.each(prepared_data.columns, x=>{
                headers[x] = T(x);
            });

            this.setState({loading: false,
                           headers: headers,
                           pageData: prepared_data});
        }).catch(() => {
            this.setState({loading: false, pageData: {}});
        });
    }

    downloadFile = (e) => {
        e.stopPropagation();
        e.preventDefault();
    }

    render() {
        if (!this.state.pageData || !this.state.pageData.columns) {
            return (
                <CardDeck>
                  <Card>
                    <Card.Header>{T("Uploaded Files")}</Card.Header>
                    <Card.Body>
                      <Spinner loading={this.state.loading}/>
                      <div className="no-content">{T("Collection did not upload files")}</div>
                    </Card.Body>
                  </Card>
                </CardDeck>
            );
        }

        let renderers = {
            // Let users directly download the file without having to
            // make a zip file.
            vfs_path: (cell, row, rowIndex) => {
                let filename = cell;

                if (filename.endsWith(".idx")) {
                    filename = filename.slice(0, -4);
                    return <>
                             <OverlayTrigger
                               delay={{show: 250, hide: 400}}
                               overlay={(props)=>{
                                   return <Tooltip {...props}>
                                            Download padded file.
                                          </Tooltip>;
                               }}>
                               <Button as="a"
                                       className="flow-file-download-button"
                                       href={api.href("/api/v1/DownloadVFSFile", {
                                           client_id: this.props.flow.client_id,
                                           fs_components: normalizeComponentList(
                                               row._Components, this.props.flow.client_id,
                                               this.props.flow.session_id),
                                           padding: true,
                                           vfs_path: filename}, {arrayFormat: 'brackets'})}>
                                 {filename} &nbsp;&nbsp; <FontAwesomeIcon icon="expand"/>
                               </Button>
                             </OverlayTrigger>
                           </>;
                }

                return <OverlayTrigger
                         delay={{show: 250, hide: 400}}
                         overlay={(props)=>{
                             return <Tooltip {...props}>
                                      Download file.
                                    </Tooltip>;
                         }}>
                         <Button as="a"
                                 className="flow-file-download-button"
                                 href={api.href("/api/v1/DownloadVFSFile", {
                                     client_id: this.props.flow.client_id,
                                     fs_components: normalizeComponentList(
                                         row._Components, this.props.flow.client_id,
                                         this.props.flow.session_id),
                                     padding: false,
                                     vfs_path: filename}, {arrayFormat: 'brackets'})}>
                           {filename}
                         </Button>
                       </OverlayTrigger>;
            },
        };

        return (
            <VeloTable
              className="col-12"
              renderers={renderers}
              rows={this.state.pageData.rows}
              headers={this.state.headers}
              columns={this.state.pageData.columns} />
        );
    }
};
