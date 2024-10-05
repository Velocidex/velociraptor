import _ from 'lodash';

import PropTypes from 'prop-types';
import React, { Component } from 'react';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import T from '../i8n/i8n.jsx';
import Form from 'react-bootstrap/Form';
import Table from 'react-bootstrap/Table';

import './dict.css';


export default class DictEditor extends Component {
    static propTypes = {
        // This needs to be an array of (k, v) tuples so we can retain
        // ordering.
        value: PropTypes.array,
        setValue: PropTypes.func.isRequired,
        valueRenderer: PropTypes.func,
        editableKeys: PropTypes.bool,
        deletableKeys: PropTypes.bool,
    }

    deleteMetadata = k=>{
        let new_obj = _.filter(this.props.value, x=>x[0] !== k);
        this.props.setValue(new_obj);
    }

    setMetadata = (k, v, replace) => {
        let new_obj = [...this.props.value];
        if(replace) {
            new_obj = _.filter(new_obj, x=>x[0] !== replace);
        }

        for(let i=0; i<new_obj.length; i++) {
            // Replace in place.
            if(new_obj[i][0] === k) {
                new_obj[i][1] = v;
                this.props.setValue(new_obj);
                return;
            }
        }

        // Key not found
        new_obj.push([k, v]);
        this.props.setValue(new_obj);
    }

    getRows = x=>{
        return x.split("\n").length;
    }

    render() {
        return (
            <Table className="paged-table dict-table">
              <thead>
                <tr>
                  <th className="metadata-control paged-table-header">
                    { this.props.editableKeys &&
                      <ButtonGroup>
                        <Button variant="default-outline" size="sm"
                                onClick={() => {
                                    this.setMetadata(
                                        T(" New Key"),
                                        T("New Value"));
                                }}
                        >
                          <FontAwesomeIcon icon="plus"/>
                        </Button>
                      </ButtonGroup>
                    }
                  </th>
                  <th className="metadata-key paged-table-header">
                    {T("Key")}
                  </th>
                  <th className="metadata-value paged-table-header">
                    {T("Value")}
                  </th>
                </tr>
              </thead>
              <tbody>
                {_.map(this.props.value, (x, idx)=>{
                    let key = x[0];
                    let value = x[1];
                    return <tr key={idx}>
                             <td className="metadata-control">
                               { this.props.deletableKeys &&
                                 <ButtonGroup>
                                   <Button variant="default-outline" size="sm"
                                           onClick={() => {
                                               // Drop the current row
                                               this.deleteMetadata(key);
                                           }}
                                   >
                                     <FontAwesomeIcon icon="trash"/>
                                   </Button>
                                 </ButtonGroup>
                               }
                             </td>
                             <td className="metadata-key">
                               {this.props.editableKeys ?
                                <Form.Control as="textarea" rows={1}
                                              value={key}
                                              onChange={e=>{
                                                  this.setMetadata(
                                                      e.currentTarget.value, value);
                                              }}
                                /> : key
                               }
                             </td>
                             <td className="metadata-value">
                               <Form.Control as="textarea" rows={this.getRows(value)}
                                             value={value}
                                             onChange={e=>{
                                                 this.setMetadata(
                                                     key, e.currentTarget.value);
                                             }}
                               />
                             </td>
                           </tr>;
                })}
              </tbody>
            </Table>
        );
    }
};
