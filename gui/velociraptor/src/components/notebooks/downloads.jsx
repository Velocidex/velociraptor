import _ from 'lodash';

import PropTypes from 'prop-types';
import React, { Component } from 'react';
import { formatColumns } from "../core/table.jsx";
import T from '../i8n/i8n.jsx';
import Card from 'react-bootstrap/Card';

import api from '../core/api-service.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';


export default class AvailableDownloads extends Component {
    static propTypes = {
        files: PropTypes.array,
    };

    getDownloadLink = (row) =>{
        var stats = row.stats || {};
        if (row.complete) {
            return <a href={api.href("/api/v1/DownloadVFSFile", {
                fs_components: stats.components,
                vfs_path: row.path,
            }, {internal: true, arrayFormat: 'brackets'})}
                      target="_blank" download
                      rel="noopener noreferrer">{row.name}</a>;
        }
        return <>
                 <FontAwesomeIcon icon="spinner" spin/>
                 <span className="button-label">{row.name}</span>
                 </>;
    }

    render() {
        if (_.isEmpty(this.props.files)) {
            return <h5 className="no-content">{T("Select a download method")}</h5>;
        }

        var columns = formatColumns(
            [ {dataField: "size", text: T("size"), sort: true, type: "mb"},
             {dataField: "date", text: T("date"), type: "timestamp"}]);

        return (
            <>
              <dl className="row">
                <dt className="col-12">{T("Available Downloads")}</dt>
              </dl>
                { _.map(this.props.files, (x, idx)=>{
                    let stats = x.stats || {};
                    return <Card key={idx}>
                             <Card.Header>
                               {this.getDownloadLink(x)}
                             </Card.Header>
                             <Card.Body>
                             <dl className="row">
                               <dt className="col-4">{T("Uncompressed")}</dt>
                               <dd className="col-8">
                                 {columns[0].formatter(
                                     stats.total_uncompressed_bytes || 0, x)}
                               </dd>

                               <dt className="col-4">{T("Compressed")}</dt>
                               <dd className="col-8">
                                 {columns[0].formatter(
                                     stats.total_compressed_bytes || 0, x)}
                               </dd>

                               <dt className="col-4">{T("Container Files")}</dt>
                               <dd className="col-8">
                                 { stats.total_container_files}
                               </dd>

                               <dt className="col-4">{T("Started")}</dt>
                               <dd className="col-8">
                                 {columns[1].formatter(stats.timestamp, x)}
                               </dd>

                               <dt className="col-4">{T("Duration (Sec)")}</dt>
                               <dd className="col-8">
                                 {stats.total_duration || 0}
                               </dd>

                               { stats.hash &&
                                 <>
                                   <dt className="col-4">{T("SHA256 Hash")}</dt>
                                   <dd className="col-8">
                                     {stats.hash}
                                   </dd>

                                 </>
                               }
                             </dl>
                             </Card.Body>
                           </Card>;
                })}
            </>
        );
    }
}
