import _ from 'lodash';

import PropTypes from 'prop-types';
import React, { Component } from 'react';
import T from '../i8n/i8n.jsx';
import Card from 'react-bootstrap/Card';
import Alert from 'react-bootstrap/Alert';
import { getFormatter } from "../core/table.jsx";
import Accordion from 'react-bootstrap/Accordion';

import api from '../core/api-service.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';


export default class AvailableDownloads extends Component {
    static propTypes = {
        files: PropTypes.array,
    };

    getDownloadLink = (row) =>{
        var stats = row.stats || {};
        var complete = stats.error;
        if (complete) {
            return <a href={api.href("/api/v1/DownloadVFSFile", {
                fs_components: stats.components,
                vfs_path: row.path,
            })}
                      target="_blank" download
                      rel="noopener noreferrer">
                     <FontAwesomeIcon icon="download" />
                     &nbsp;
                     {row.name}
                   </a>;
        }
        return <>
                 <FontAwesomeIcon icon="spinner" spin/>
                 <span className="button-label">{row.name}</span>
                 </>;
    }

    renderActiveMembers = (stats)=>{
        let mb = getFormatter("mb");
        return <Accordion>
                 <Accordion.Item eventKey={1} key={1}>
                   <Accordion.Header>
                     {T("Active Members")}
                   </Accordion.Header>
                   <Accordion.Body>
                     <table>
                       <thead>
                         <tr>
                           <th>{T("Name")}</th>
                           <th>{T("Uncompressed")}</th>
                           <th>{T("Compressed")}</th>
                         </tr>
                       </thead>
                       <tbody>
                         {_.map(stats.active_members, (x, idx)=>{
                             return <tr key={idx}>
                              <td>{x.name}</td>
                              <td>{mb(x.uncompressed_size || 0, x)}</td>
                              <td>{mb(x.compressed_size || 0, x)}</td>
                            </tr>;
                         })}
                       </tbody>
                     </table>
                   </Accordion.Body>
                 </Accordion.Item>
               </Accordion>;
    };

    render() {
        if (_.isEmpty(this.props.files)) {
            return <h5 className="no-content">{T("Select a download method")}</h5>;
        }

        let mb = getFormatter("mb");
        let ts = getFormatter("timestamp");

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
                                 { stats.error && stats.error !== "Complete" &&
                                   <dd className="col-12">
                                     <Alert variant="danger">
                                       {T(stats.error)}
                                   </Alert>
                                   </dd>
                                 }
                               <dt className="col-4">{T("Uncompressed")}</dt>
                               <dd className="col-8">
                                 {mb(stats.total_uncompressed_bytes || 0, x)}
                               </dd>

                               <dt className="col-4">{T("Compressed")}</dt>
                               <dd className="col-8">
                                 {mb(stats.total_compressed_bytes || 0, x)}
                               </dd>

                               <dt className="col-4">{T("Container Files")}</dt>
                               <dd className="col-8">
                                 { stats.total_container_files}
                               </dd>

                               <dt className="col-4">{T("Started")}</dt>
                               <dd className="col-8">
                                 {ts(stats.timestamp, x)}
                               </dd>

                               <dt className="col-4">{T("Duration (Sec)")}</dt>
                               <dd className="col-8">
                                 {stats.total_duration || 0}
                               </dd>

                                 { stats.active_members && this.renderActiveMembers(stats) }
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
